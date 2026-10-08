package companion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestResolvedMusicMetadataPrefersMusicFields(t *testing.T) {
	track, artist := resolvedMusicMetadata(ytOutput{
		Title: "Artist - Song (Official Video)", Track: "Song", Artist: "Artist",
		Creator: "Creator", Uploader: "Label",
	})
	if track != "Song" || artist != "Artist" {
		t.Fatalf("unexpected music metadata: track=%q artist=%q", track, artist)
	}
}

func TestResolvedMusicMetadataFallsBackToDisplayMetadata(t *testing.T) {
	track, artist := resolvedMusicMetadata(ytOutput{Title: "Song", Creator: "Artist", Uploader: "Label"})
	if track != "Song" || artist != "Artist" {
		t.Fatalf("unexpected fallback metadata: track=%q artist=%q", track, artist)
	}
}

func TestStreamExpiry(t *testing.T) {
	tests := []struct {
		name string
		urls []string
		want int64
	}{
		{name: "query", urls: []string{"https://example.test/video?expire=200"}, want: 200},
		{name: "path", urls: []string{"https://manifest.googlevideo.com/api/manifest/expire/300/playlist.m3u8"}, want: 300},
		{name: "earliest across URLs", urls: []string{"https://example.test/expire/500/video", "https://example.test/audio?expire=400"}, want: 400},
		{name: "earliest within URL", urls: []string{"https://example.test/expire/700/video?expire=600"}, want: 600},
		{name: "invalid values ignored", urls: []string{"not a URL", "https://example.test/expire/nope/video?expire="}, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := streamExpiry(test.urls...); got != test.want {
				t.Fatalf("streamExpiry() = %d, want %d", got, test.want)
			}
		})
	}
}

func testResolver(t *testing.T) *Resolver {
	t.Helper()
	store, err := newConfigStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return newResolver(t.TempDir(), store)
}

func ytDLPReleaseServer(t *testing.T, binary []byte, serveBinary http.HandlerFunc) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(binary)
	asset := "yt-dlp.exe"
	if runtime.GOARCH == "arm64" {
		asset = "yt-dlp_arm64.exe"
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA2-256SUMS":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), asset)
		case "/" + asset:
			serveBinary(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestResolveWithoutYTDLPFailsFastAndInstallsInBackground(t *testing.T) {
	binary := []byte("fake yt-dlp binary")
	resolver := testResolver(t)
	resolver.baseURL = ytDLPReleaseServer(t, binary, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(binary)
	}).URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.Resolve(ctx, "wEZFS5aert8", false); !errors.Is(err, errYTDLPInstalling) {
		t.Fatalf("Resolve() error = %v, want errYTDLPInstalling", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !resolver.Ready() || resolver.installing.Load() {
		if time.Now().After(deadline) {
			t.Fatal("background install did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if saved, err := os.ReadFile(resolver.path); err != nil || string(saved) != string(binary) {
		t.Fatalf("installed binary = %q, %v", saved, err)
	}
}

func TestYTDLPDownloadOutlastsFixedClientTimeoutWhileProgressing(t *testing.T) {
	previous := downloadStallTimeout
	downloadStallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { downloadStallTimeout = previous })
	binary := []byte(strings.Repeat("y", 8))
	resolver := testResolver(t)
	resolver.baseURL = ytDLPReleaseServer(t, binary, func(w http.ResponseWriter, _ *http.Request) {
		for _, b := range binary {
			_, _ = w.Write([]byte{b})
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}).URL
	if err := resolver.Ensure(context.Background()); err != nil {
		t.Fatalf("slow but steady download failed: %v", err)
	}
}

func TestYTDLPDownloadFailsWhenStalled(t *testing.T) {
	previous := downloadStallTimeout
	downloadStallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { downloadStallTimeout = previous })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	resolver := testResolver(t)
	resolver.baseURL = ytDLPReleaseServer(t, []byte("complete"), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("part"))
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}).URL
	if err := resolver.Ensure(context.Background()); !errors.Is(err, errDownloadStalled) {
		t.Fatalf("Ensure() error = %v, want errDownloadStalled", err)
	}
	if resolver.Ready() {
		t.Fatal("stalled download left a binary behind")
	}
}

func TestEnsureKeepsBinaryWhenRequestIsCancelled(t *testing.T) {
	resolver := testResolver(t)
	if err := os.WriteFile(resolver.path, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := resolver.Ensure(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ensure() error = %v, want context.Canceled", err)
	}
	if !resolver.Ready() {
		t.Fatal("cancelled version check deleted yt-dlp")
	}
}
