package main

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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	companionReleaseAPI = "https://api.github.com/repos/Garruvex/zzz-wallpaper-companion/releases/latest"
	updateManifestName  = "zzz-wallpaper-companion-update.json"
	maxUpdateSize       = 64 << 20
)

type updateAsset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type updateManifest struct {
	Version     string                 `json:"version"`
	ProtocolMin int                    `json:"protocolMin"`
	ProtocolMax int                    `json:"protocolMax"`
	Assets      map[string]updateAsset `json:"assets"`
}

type pendingUpdate struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type githubRelease struct {
	Draft      bool `json:"draft"`
	Prerelease bool `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	UpdateStaged   bool
	AlreadyCurrent bool
}

type UpdateManager struct {
	mu         sync.Mutex
	dataDir    string
	httpClient *http.Client
	releaseAPI string
}

func newUpdateManager(dataDir string) *UpdateManager {
	return &UpdateManager{
		dataDir:    dataDir,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		releaseAPI: companionReleaseAPI,
	}
}

func (m *UpdateManager) CheckAndStage(ctx context.Context) (UpdateResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := UpdateResult{CurrentVersion: version}
	release, err := m.fetchRelease(ctx)
	if err != nil {
		return result, err
	}
	manifestURL := releaseAssetURL(release, updateManifestName)
	if manifestURL == "" {
		return result, errors.New("latest release has no update manifest")
	}
	if !officialGitHubDownload(manifestURL) {
		return result, errors.New("update manifest URL is not an official GitHub download")
	}
	var manifest updateManifest
	if err := m.fetchJSON(ctx, manifestURL, 1<<20, &manifest); err != nil {
		return result, fmt.Errorf("update manifest: %w", err)
	}
	result.LatestVersion = manifest.Version
	if err := validateUpdateManifest(manifest); err != nil {
		return result, err
	}
	if compareVersions(manifest.Version, version) <= 0 {
		result.AlreadyCurrent = true
		return result, nil
	}
	asset, ok := manifest.Assets["windows-"+runtime.GOARCH]
	if !ok {
		return result, fmt.Errorf("release %s has no Windows %s build", manifest.Version, runtime.GOARCH)
	}
	downloadURL := releaseAssetURL(release, asset.Name)
	if downloadURL == "" {
		return result, fmt.Errorf("release asset %s is missing", asset.Name)
	}
	pendingPath := filepath.Join(m.dataDir, "companion-update.exe")
	if err := m.downloadVerified(ctx, downloadURL, asset.SHA256, pendingPath); err != nil {
		return result, err
	}
	metadata, _ := json.MarshalIndent(pendingUpdate{Version: manifest.Version, SHA256: strings.ToLower(asset.SHA256)}, "", "  ")
	if err := os.WriteFile(filepath.Join(m.dataDir, "companion-update.json"), append(metadata, '\n'), 0o600); err != nil {
		_ = os.Remove(pendingPath)
		return result, err
	}
	result.UpdateStaged = true
	return result, nil
}

func (m *UpdateManager) fetchRelease(ctx context.Context) (githubRelease, error) {
	var release githubRelease
	if err := m.fetchJSON(ctx, m.releaseAPI, 2<<20, &release); err != nil {
		return release, err
	}
	if release.Draft || release.Prerelease {
		return release, errors.New("latest release is not a stable published release")
	}
	return release, nil
}

func (m *UpdateManager) fetchJSON(ctx context.Context, rawURL string, limit int64, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "zzz-wallpaper-companion/"+version)
	response, err := m.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, limit)).Decode(target)
}

func (m *UpdateManager) downloadVerified(ctx context.Context, rawURL, expected, destination string) error {
	if !officialGitHubDownload(rawURL) {
		return errors.New("update asset URL is not an official GitHub download")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "zzz-wallpaper-companion/"+version)
	response, err := m.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxUpdateSize {
		return errors.New("update executable exceeds the size limit")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	temporary := destination + ".download"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, maxUpdateSize+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written > maxUpdateSize {
		_ = os.Remove(temporary)
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		return errors.New("update executable exceeds the size limit")
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		_ = os.Remove(temporary)
		return fmt.Errorf("update checksum mismatch: expected %s, got %s", expected, actual)
	}
	_ = os.Remove(destination)
	return os.Rename(temporary, destination)
}

func validateUpdateManifest(manifest updateManifest) error {
	if compareVersions(manifest.Version, "0.0.0") <= 0 {
		return errors.New("update manifest has an invalid version")
	}
	if manifest.ProtocolMin <= 0 || manifest.ProtocolMax < manifest.ProtocolMin {
		return errors.New("update manifest has an invalid protocol range")
	}
	if manifest.ProtocolMax < protocolMinVersion || manifest.ProtocolMin > protocolMaxVersion {
		return fmt.Errorf("companion %s is incompatible with protocol %d-%d", manifest.Version, protocolMinVersion, protocolMaxVersion)
	}
	for platform, asset := range manifest.Assets {
		if asset.Name == "" || len(asset.SHA256) != 64 {
			return fmt.Errorf("update manifest has an invalid %s asset", platform)
		}
		if _, err := hex.DecodeString(asset.SHA256); err != nil {
			return fmt.Errorf("update manifest has an invalid %s checksum", platform)
		}
	}
	return nil
}

func releaseAssetURL(release githubRelease, name string) string {
	for _, asset := range release.Assets {
		if asset.Name == name && asset.Size <= maxUpdateSize {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func officialGitHubDownload(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && strings.EqualFold(parsed.Hostname(), "github.com")
}

func compareVersions(left, right string) int {
	parse := func(value string) [3]int {
		value = strings.TrimPrefix(strings.TrimSpace(value), "v")
		value = strings.SplitN(value, "-", 2)[0]
		parts := strings.Split(value, ".")
		var result [3]int
		for index := 0; index < len(parts) && index < len(result); index++ {
			result[index], _ = strconv.Atoi(parts[index])
		}
		return result
	}
	a, b := parse(left), parse(right)
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}

func pendingUpdatePaths(dataDir string) (executable, metadata string) {
	return filepath.Join(dataDir, "companion-update.exe"), filepath.Join(dataDir, "companion-update.json")
}

func maybeLaunchPendingUpdate(dataDir string) bool {
	pendingExecutable, pendingMetadata := pendingUpdatePaths(dataDir)
	metadataBody, err := os.ReadFile(pendingMetadata)
	if err != nil {
		return false
	}
	var metadata pendingUpdate
	if json.Unmarshal(metadataBody, &metadata) != nil || compareVersions(metadata.Version, version) <= 0 {
		_ = os.Remove(pendingMetadata)
		_ = os.Remove(pendingExecutable)
		return false
	}
	actual, err := fileSHA256(pendingExecutable)
	if err != nil || !strings.EqualFold(actual, metadata.SHA256) {
		_ = os.Remove(pendingMetadata)
		_ = os.Remove(pendingExecutable)
		return false
	}
	target, err := os.Executable()
	if err != nil {
		return false
	}
	command := exec.Command(pendingExecutable, "--apply-update", target, dataDir)
	hideCommandWindow(command)
	if err := command.Start(); err != nil {
		return false
	}
	return true
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func runUpdateHelper(target, dataDir string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	backup := target + ".previous"
	deadline := time.Now().Add(20 * time.Second)
	for {
		_ = os.Remove(backup)
		err = os.Rename(target, backup)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("could not replace running companion: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err := copyExecutable(self, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	marker := filepath.Join(dataDir, fmt.Sprintf("update-confirm-%d", time.Now().UnixNano()))
	command := exec.Command(target, "--confirm-update", marker)
	hideCommandWindow(command)
	command.Env = append(os.Environ(), "ZZZ_COMPANION_DATA_DIR="+dataDir)
	if err := command.Start(); err != nil {
		_ = os.Remove(target)
		_ = os.Rename(backup, target)
		return err
	}
	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			_ = os.Remove(marker)
			_, metadata := pendingUpdatePaths(dataDir)
			_ = os.Remove(metadata)
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = command.Process.Kill()
	_ = command.Wait()
	_ = os.Remove(target)
	if err := os.Rename(backup, target); err != nil {
		return fmt.Errorf("update failed and rollback failed: %w", err)
	}
	rollback := exec.Command(target)
	hideCommandWindow(rollback)
	rollback.Env = append(os.Environ(), "ZZZ_COMPANION_DATA_DIR="+dataDir)
	_ = rollback.Start()
	return errors.New("updated companion did not confirm startup; rollback restored")
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(destination)
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	}
	return nil
}
