package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const nightlyBaseURL = "https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download"
const stableBaseURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download"

type Resolver struct {
	mu         sync.Mutex
	path       string
	httpClient *http.Client
	settings   *ConfigStore
}

type resolvedMedia struct {
	VideoURL    string `json:"videoUrl"`
	AudioURL    string `json:"audioUrl"`
	VideoFormat string `json:"videoFormatId"`
	AudioFormat string `json:"audioFormatId"`
	Extension   string `json:"extension"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	VideoCodec  string `json:"videoCodec,omitempty"`
	AudioCodec  string `json:"audioCodec,omitempty"`
	Title       string `json:"title,omitempty"`
	IsLive      bool   `json:"isLive"`
	Muxed       bool   `json:"muxed"`
	ExpiresAt   int64  `json:"expiresAt,omitempty"`
}

type playlistItem struct {
	ID       string `json:"id"`
	Title    string `json:"title,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type ytFormat struct {
	URL      string `json:"url"`
	FormatID string `json:"format_id"`
	VCodec   string `json:"vcodec"`
	ACodec   string `json:"acodec"`
	Ext      string `json:"ext"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type ytOutput struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	Duration         float64    `json:"duration"`
	IsLive           bool       `json:"is_live"`
	URL              string     `json:"url"`
	FormatID         string     `json:"format_id"`
	Ext              string     `json:"ext"`
	VCodec           string     `json:"vcodec"`
	ACodec           string     `json:"acodec"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	RequestedFormats []ytFormat `json:"requested_formats"`
	Entries          []ytOutput `json:"entries"`
}

func newResolver(dataDir string, settings *ConfigStore) *Resolver {
	return &Resolver{
		path:       filepath.Join(dataDir, "yt-dlp.exe"),
		httpClient: &http.Client{Timeout: 2 * time.Minute},
		settings:   settings,
	}
}

func (r *Resolver) Ensure(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := os.Stat(r.path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return r.download(ctx)
}

func (r *Resolver) Update(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := os.Stat(r.path); errors.Is(err, os.ErrNotExist) {
		return r.download(ctx)
	}
	channel := r.settings.Get().UpdateChannel
	cmd := exec.CommandContext(ctx, r.path, "--update-to", channel)
	hideCommandWindow(cmd)
	cmd.Dir = filepath.Dir(r.path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yt-dlp update failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (r *Resolver) download(ctx context.Context) error {
	baseURL := nightlyBaseURL
	if r.settings.Get().UpdateChannel == "stable" {
		baseURL = stableBaseURL
	}
	asset := "yt-dlp.exe"
	if runtime.GOARCH == "arm64" {
		asset = "yt-dlp_arm64.exe"
	}
	expected, err := r.fetchChecksum(ctx, baseURL+"/SHA2-256SUMS", asset)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/"+asset, nil)
	if err != nil {
		return err
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("yt-dlp download returned HTTP %d", response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	tmp := r.path + ".download"
	file, err := os.Create(tmp)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(file, hash), response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		_ = os.Remove(tmp)
		return fmt.Errorf("yt-dlp checksum mismatch: expected %s, got %s", expected, actual)
	}
	return os.Rename(tmp, r.path)
}

func (r *Resolver) fetchChecksum(ctx context.Context, url, asset string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksum download returned HTTP %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == asset {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum published for %s", asset)
}

func (r *Resolver) Resolve(ctx context.Context, id string) (resolvedMedia, error) {
	if err := r.Ensure(ctx); err != nil {
		return resolvedMedia{}, err
	}
	height := r.settings.Get().MaxHeight
	format := fmt.Sprintf("bv[ext=webm][vcodec^=vp9][height<=%d]+ba[ext=webm][acodec^=opus]/best[height<=%d]", height, height)
	output, err := r.run(ctx, "--no-playlist", "--no-warnings", "--socket-timeout", "15", "--extractor-retries", "2", "--dump-single-json", "--format", format, "https://www.youtube.com/watch?v="+id)
	if err != nil {
		return resolvedMedia{}, err
	}
	var metadata ytOutput
	if err := json.Unmarshal(output, &metadata); err != nil {
		return resolvedMedia{}, err
	}
	var video, audio *ytFormat
	for i := range metadata.RequestedFormats {
		item := &metadata.RequestedFormats[i]
		if video == nil && item.VCodec != "" && item.VCodec != "none" {
			video = item
		}
		if audio == nil && item.ACodec != "" && item.ACodec != "none" {
			audio = item
		}
	}
	if video != nil && audio != nil && video.URL != "" && audio.URL != "" {
		return resolvedMedia{VideoURL: video.URL, AudioURL: audio.URL, VideoFormat: video.FormatID, AudioFormat: audio.FormatID, Extension: "webm", Width: video.Width, Height: video.Height, VideoCodec: video.VCodec, AudioCodec: audio.ACodec, Title: metadata.Title, IsLive: metadata.IsLive, ExpiresAt: streamExpiry(video.URL, audio.URL)}, nil
	}
	if metadata.URL == "" || metadata.VCodec == "" || metadata.VCodec == "none" {
		return resolvedMedia{}, errors.New("no browser-playable stream was found")
	}
	return resolvedMedia{VideoURL: metadata.URL, VideoFormat: metadata.FormatID, Extension: metadata.Ext, Width: metadata.Width, Height: metadata.Height, VideoCodec: metadata.VCodec, AudioCodec: metadata.ACodec, Title: metadata.Title, IsLive: metadata.IsLive, Muxed: true, ExpiresAt: streamExpiry(metadata.URL)}, nil
}

func streamExpiry(urls ...string) int64 {
	var earliest int64
	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		expires, err := strconv.ParseInt(parsed.Query().Get("expire"), 10, 64)
		if err == nil && expires > 0 && (earliest == 0 || expires < earliest) {
			earliest = expires
		}
	}
	return earliest
}

func (r *Resolver) Playlist(ctx context.Context, id string) ([]playlistItem, error) {
	if err := r.Ensure(ctx); err != nil {
		return nil, err
	}
	output, err := r.run(ctx, "--flat-playlist", "--dump-single-json", "--playlist-end", "500", "https://www.youtube.com/playlist?list="+id)
	if err != nil {
		return nil, err
	}
	var metadata ytOutput
	if err := json.Unmarshal(output, &metadata); err != nil {
		return nil, err
	}
	items := make([]playlistItem, 0, len(metadata.Entries))
	for _, entry := range metadata.Entries {
		if validYouTubeID(entry.ID) {
			items = append(items, playlistItem{ID: entry.ID, Title: entry.Title, Duration: int(entry.Duration)})
		}
	}
	return items, nil
}

func (r *Resolver) run(ctx context.Context, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmd := exec.CommandContext(ctx, r.path, args...)
	hideCommandWindow(cmd)
	cmd.Dir = filepath.Dir(r.path)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("yt-dlp failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}
	return output, nil
}
