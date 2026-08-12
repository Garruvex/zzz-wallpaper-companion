package companion

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const ffmpegReleaseURL = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest"

type FFmpegManager struct {
	mu         sync.Mutex
	path       string
	httpClient *http.Client
	ready      bool
}

func newFFmpegManager(dataDir string) *FFmpegManager {
	return &FFmpegManager{
		path:       filepath.Join(dataDir, "ffmpeg.exe"),
		httpClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (m *FFmpegManager) Ready() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ready
}

func (m *FFmpegManager) Ensure(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Stat(m.path); err == nil {
		cmd := exec.CommandContext(ctx, m.path, "-version")
		hideCommandWindow(cmd)
		if err := cmd.Run(); err == nil {
			m.ready = true
			return nil
		}
		_ = os.Remove(m.path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := m.download(ctx); err != nil {
		return err
	}
	m.ready = true
	return nil
}

func (m *FFmpegManager) download(ctx context.Context) error {
	asset := "ffmpeg-master-latest-win64-lgpl.zip"
	if runtime.GOARCH == "arm64" {
		asset = "ffmpeg-master-latest-winarm64-lgpl.zip"
	}
	expected, err := m.fetchChecksum(ctx, asset)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, ffmpegReleaseURL+"/"+asset, nil)
	if err != nil {
		return err
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("FFmpeg download returned HTTP %d", response.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	archivePath := m.path + ".zip.download"
	archive, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(archive, hash), response.Body)
	closeErr := archive.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(archivePath)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		_ = os.Remove(archivePath)
		return fmt.Errorf("FFmpeg checksum mismatch: expected %s, got %s", expected, actual)
	}
	if err := extractFFmpeg(archivePath, m.path); err != nil {
		_ = os.Remove(archivePath)
		return err
	}
	_ = os.Remove(archivePath)
	return nil
}

func (m *FFmpegManager) fetchChecksum(ctx context.Context, asset string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, ffmpegReleaseURL+"/checksums.sha256", nil)
	if err != nil {
		return "", err
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("FFmpeg checksum download returned HTTP %d", response.StatusCode)
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

func extractFFmpeg(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if !strings.EqualFold(filepath.Base(filepath.FromSlash(file.Name)), "ffmpeg.exe") {
			continue
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		tmp := destination + ".download"
		output, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		input.Close()
		closeErr := output.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmp)
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}
		return os.Rename(tmp, destination)
	}
	return errors.New("FFmpeg executable was not found in downloaded archive")
}
