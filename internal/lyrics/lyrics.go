package lyrics

import (
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
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxLyricsResponseBytes = 1 << 20

type LyricsRequest struct {
	Artist   string  `json:"artist"`
	Title    string  `json:"title"`
	Album    string  `json:"album,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

type LyricLine struct {
	TimeMS int64  `json:"timeMs"`
	Text   string `json:"text"`
}

type LyricsResponse struct {
	Status       string      `json:"status"`
	TrackKey     string      `json:"trackKey"`
	Source       string      `json:"source,omitempty"`
	Synced       bool        `json:"synced"`
	Instrumental bool        `json:"instrumental,omitempty"`
	Lines        []LyricLine `json:"lines,omitempty"`
	PlainLines   []string    `json:"plainLines,omitempty"`
}

type LyricsService struct {
	cacheDir      string
	clientVersion string
	client        *http.Client
	mu            sync.RWMutex
	memory        map[string]LyricsResponse
	onlineMu      sync.Mutex
	requestMu     sync.Mutex
	currentKey    string
	cancelCurrent context.CancelFunc
	requestID     uint64
}

func NewService(dataDir, clientVersion string) *LyricsService {
	return &LyricsService{
		cacheDir:      filepath.Join(dataDir, "lyrics"),
		clientVersion: clientVersion,
		client:        &http.Client{Timeout: 8 * time.Second},
		memory:        make(map[string]LyricsResponse),
	}
}

func lyricsTrackKey(r LyricsRequest) string {
	normalized := strings.ToLower(strings.Join([]string{strings.TrimSpace(r.Artist), strings.TrimSpace(r.Title), strings.TrimSpace(r.Album), strconv.Itoa(int(r.Duration + .5))}, "|"))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:16])
}

func (s *LyricsService) Lookup(ctx context.Context, req LyricsRequest) (LyricsResponse, error) {
	req.Artist, req.Title, req.Album = strings.TrimSpace(req.Artist), strings.TrimSpace(req.Title), strings.TrimSpace(req.Album)
	if req.Title == "" || len(req.Title) > 500 || len(req.Artist) > 500 || len(req.Album) > 500 || req.Duration < 0 {
		return LyricsResponse{}, errors.New("invalid track metadata")
	}
	key := lyricsTrackKey(req)
	if cached, ok := s.cached(key); ok {
		return cached, nil
	}
	lookupCtx, cancel := context.WithCancel(ctx)
	s.requestMu.Lock()
	if s.cancelCurrent != nil {
		s.cancelCurrent()
	}
	s.requestID++
	requestID := s.requestID
	s.currentKey, s.cancelCurrent = key, cancel
	s.requestMu.Unlock()
	defer func() {
		cancel()
		s.requestMu.Lock()
		if s.requestID == requestID {
			s.currentKey, s.cancelCurrent = "", nil
		}
		s.requestMu.Unlock()
	}()

	select {
	case <-time.After(2 * time.Second):
	case <-lookupCtx.Done():
		return LyricsResponse{}, lookupCtx.Err()
	}
	if cached, ok := s.cached(key); ok {
		return cached, nil
	}

	s.onlineMu.Lock()
	defer s.onlineMu.Unlock()
	if cached, ok := s.cached(key); ok {
		return cached, nil
	}

	result, err := s.lookupLRCLIB(lookupCtx, req, key)
	if err != nil || result.Status != "ready" {
		result, err = s.lookupNetEase(lookupCtx, req, key)
	}
	if err != nil {
		return LyricsResponse{Status: "error", TrackKey: key}, err
	}
	if result.Status == "" {
		result = LyricsResponse{Status: "not_found", TrackKey: key}
	}
	s.store(key, result)
	return result, nil
}

func (s *LyricsService) cached(key string) (LyricsResponse, bool) {
	s.mu.RLock()
	value, ok := s.memory[key]
	s.mu.RUnlock()
	if ok {
		return value, true
	}
	body, err := os.ReadFile(filepath.Join(s.cacheDir, key+".json"))
	if err != nil || len(body) > maxLyricsResponseBytes {
		return LyricsResponse{}, false
	}
	var result LyricsResponse
	if json.Unmarshal(body, &result) != nil || result.TrackKey != key {
		return LyricsResponse{}, false
	}
	s.mu.Lock()
	s.memory[key] = result
	s.mu.Unlock()
	return result, true
}

func (s *LyricsService) store(key string, result LyricsResponse) {
	s.mu.Lock()
	s.memory[key] = result
	s.mu.Unlock()
	if result.Status != "ready" && result.Status != "instrumental" {
		return
	}
	body, err := json.Marshal(result)
	if err != nil || len(body) > maxLyricsResponseBytes {
		return
	}
	if os.MkdirAll(s.cacheDir, 0o700) == nil {
		_ = os.WriteFile(filepath.Join(s.cacheDir, key+".json"), body, 0o600)
	}
}

func (s *LyricsService) lookupLRCLIB(ctx context.Context, track LyricsRequest, key string) (LyricsResponse, error) {
	endpoint, _ := url.Parse("https://lrclib.net/api/get")
	q := endpoint.Query()
	q.Set("track_name", track.Title)
	q.Set("artist_name", track.Artist)
	if track.Album != "" {
		q.Set("album_name", track.Album)
	}
	if track.Duration > 0 {
		q.Set("duration", strconv.Itoa(int(track.Duration+.5)))
	}
	endpoint.RawQuery = q.Encode()
	var body struct {
		Synced, Plain string
		Instrumental  bool
	}
	var raw struct {
		SyncedLyrics string `json:"syncedLyrics"`
		PlainLyrics  string `json:"plainLyrics"`
		Instrumental bool   `json:"instrumental"`
	}
	status, err := s.getJSON(ctx, endpoint.String(), "zzz-wallpaper-companion/"+s.clientVersion, "", &raw)
	if err != nil {
		return LyricsResponse{}, err
	}
	if status == http.StatusNotFound {
		return LyricsResponse{Status: "not_found", TrackKey: key}, nil
	}
	body.Synced, body.Plain, body.Instrumental = raw.SyncedLyrics, raw.PlainLyrics, raw.Instrumental
	return makeLyricsResponse(key, "lrclib", body.Synced, body.Plain, body.Instrumental), nil
}

func (s *LyricsService) lookupNetEase(ctx context.Context, track LyricsRequest, key string) (LyricsResponse, error) {
	endpoint, _ := url.Parse("https://music.163.com/api/search/get")
	q := endpoint.Query()
	q.Set("s", strings.TrimSpace(track.Title+" "+track.Artist))
	q.Set("type", "1")
	q.Set("limit", "5")
	endpoint.RawQuery = q.Encode()
	var search struct {
		Result struct {
			Songs []struct {
				ID      int64 `json:"id"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"songs"`
		} `json:"result"`
	}
	_, err := s.getJSON(ctx, endpoint.String(), "Mozilla/5.0", "https://music.163.com/", &search)
	if err != nil {
		return LyricsResponse{}, err
	}
	var id int64
	for _, song := range search.Result.Songs {
		for _, artist := range song.Artists {
			if containsFold(track.Artist, artist.Name) || containsFold(artist.Name, track.Artist) {
				id = song.ID
				break
			}
		}
		if id != 0 {
			break
		}
	}
	if id == 0 {
		return LyricsResponse{Status: "not_found", TrackKey: key}, nil
	}
	lyricsURL := fmt.Sprintf("https://music.163.com/api/song/lyric?id=%d&lv=1&kv=1&tv=-1", id)
	var lyric struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	_, err = s.getJSON(ctx, lyricsURL, "Mozilla/5.0", "https://music.163.com/", &lyric)
	if err != nil {
		return LyricsResponse{}, err
	}
	return makeLyricsResponse(key, "netease", lyric.LRC.Lyric, "", false), nil
}

func (s *LyricsService) getJSON(ctx context.Context, endpoint, userAgent, referer string, target any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("lyrics provider returned HTTP %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxLyricsResponseBytes))
	err = decoder.Decode(target)
	return resp.StatusCode, err
}

var lrcTimestamp = regexp.MustCompile(`\[(\d+):(\d+(?:\.\d+)?)\]`)

func makeLyricsResponse(key, source, synced, plain string, instrumental bool) LyricsResponse {
	if instrumental {
		return LyricsResponse{Status: "instrumental", TrackKey: key, Source: source, Instrumental: true}
	}
	lines := parseLRC(synced)
	if len(lines) > 0 {
		return LyricsResponse{Status: "ready", TrackKey: key, Source: source, Synced: true, Lines: lines}
	}
	plainLines := cleanPlainLines(plain)
	if len(plainLines) > 0 {
		return LyricsResponse{Status: "ready", TrackKey: key, Source: source, PlainLines: plainLines}
	}
	return LyricsResponse{Status: "not_found", TrackKey: key}
}

func parseLRC(text string) []LyricLine {
	result := make([]LyricLine, 0)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		matches := lrcTimestamp.FindAllStringSubmatch(raw, -1)
		if len(matches) == 0 {
			continue
		}
		lyric := strings.TrimSpace(lrcTimestamp.ReplaceAllString(raw, ""))
		if len(lyric) > 2000 {
			lyric = lyric[:2000]
		}
		for _, match := range matches {
			mins, _ := strconv.Atoi(match[1])
			secs, _ := strconv.ParseFloat(match[2], 64)
			ms := int64(float64(mins)*60000 + secs*1000)
			if ms >= 0 {
				result = append(result, LyricLine{TimeMS: ms, Text: lyric})
			}
			if len(result) >= 10000 {
				break
			}
		}
		if len(result) >= 10000 {
			break
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].TimeMS < result[j].TimeMS })
	return result
}

func cleanPlainLines(text string) []string {
	result := make([]string, 0)
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 2000 {
			line = line[:2000]
		}
		result = append(result, line)
		if len(result) >= 10000 {
			break
		}
	}
	return result
}

func containsFold(a, b string) bool {
	return b != "" && strings.Contains(strings.ToLower(a), strings.ToLower(b))
}
