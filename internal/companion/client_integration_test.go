package companion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWallpaperClientIntegration(t *testing.T) {
	wallpaper := os.Getenv("COMPANION_WALLPAPER_PATH")
	if wallpaper == "" {
		t.Skip("set COMPANION_WALLPAPER_PATH to the wallpaper checkout")
	}
	s := testServer(t)
	var tileCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tileCalls.Add(1)
		if r.URL.Path != "/2/1/1.png" {
			t.Errorf("unexpected generated tile path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=604800")
		_, _ = w.Write(testPNG)
	}))
	defer provider.Close()
	s.mapTiles.baseURL = provider.URL
	host := httptest.NewServer(s.http.Handler)
	defer host.Close()
	node := os.Getenv("DOCS_NODE")
	if node == "" {
		node = "node"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join("..", "..", "tools", "wallpaper-client.cjs"), host.URL, wallpaper)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wallpaper client: %v\n%s", err, output)
	}
	if tileCalls.Load() != 1 {
		t.Fatalf("generated tile requests bypassed cache: %d upstream calls", tileCalls.Load())
	}
	var ids map[string]string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "LEASE_IDS=") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "LEASE_IDS=")), &ids); err != nil {
				t.Fatal(err)
			}
		}
	}
	if ids["released"] == "" || ids["retained"] == "" || ids["health"] == "" {
		t.Fatal("missing lease verification IDs")
	}
	if s.streamLeaseAlive(ids["released"]) || !s.streamLeaseAlive(ids["retained"]) || s.streamLeaseAlive(ids["health"]) {
		t.Fatal("playback release or shared health altered another session's retained lease")
	}
	s.cancelAllStreams()
	t.Log(string(output))
}
