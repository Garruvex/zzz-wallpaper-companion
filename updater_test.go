package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestCompareVersions(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"1.1.1", "1.1.0", 1},
		{"v1.1.0", "1.1.0", 0},
		{"1.0.9", "1.1.0", -1},
	} {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestValidateUpdateManifestRejectsIncompatibleProtocol(t *testing.T) {
	manifest := updateManifest{Version: "9.0.0", ProtocolMin: protocolMaxVersion + 1, ProtocolMax: protocolMaxVersion + 1}
	if err := validateUpdateManifest(manifest); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("expected incompatible protocol error, got %v", err)
	}
}

func TestCheckAndStageVerifiedUpdate(t *testing.T) {
	oldVersion := version
	version = "1.1.0"
	defer func() { version = oldVersion }()
	payload := []byte("mock executable")
	digest := sha256.Sum256(payload)
	checksum := hex.EncodeToString(digest[:])
	assetName := "zzz-wallpaper-companion-windows-" + runtime.GOARCH + ".exe"
	manifest := `{"version":"1.1.1","protocolMin":2,"protocolMax":2,"assets":{"windows-` + runtime.GOARCH + `":{"name":"` + assetName + `","sha256":"` + checksum + `"}}}`
	release := `{"draft":false,"prerelease":false,"assets":[` +
		`{"name":"` + updateManifestName + `","browser_download_url":"https://github.com/example/manifest","size":` + strconv.Itoa(len(manifest)) + `},` +
		`{"name":"` + assetName + `","browser_download_url":"https://github.com/example/executable","size":` + strconv.Itoa(len(payload)) + `}]}`

	manager := newUpdateManager(t.TempDir())
	manager.releaseAPI = "https://api.example/release"
	manager.httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := release
		if strings.Contains(request.URL.Path, "manifest") {
			body = manifest
		} else if strings.Contains(request.URL.Path, "executable") {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(payload))), ContentLength: int64(len(payload)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Header: make(http.Header)}, nil
	})
	result, err := manager.CheckAndStage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateStaged || result.LatestVersion != "1.1.1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	staged, err := os.ReadFile(filepath.Join(manager.dataDir, "companion-update.exe"))
	if err != nil || string(staged) != string(payload) {
		t.Fatalf("unexpected staged executable: %q, %v", staged, err)
	}
}

func TestMaybeLaunchPendingUpdateRemovesOrphanExecutable(t *testing.T) {
	dataDir := t.TempDir()
	executable, _ := pendingUpdatePaths(dataDir)
	if err := os.WriteFile(executable, []byte("old update helper"), 0o700); err != nil {
		t.Fatal(err)
	}

	if maybeLaunchPendingUpdate(dataDir) {
		t.Fatal("orphan update helper must not launch")
	}
	if _, err := os.Stat(executable); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan update helper was not removed: %v", err)
	}
}
