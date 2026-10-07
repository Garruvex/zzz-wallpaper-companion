package companion

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var testPNG = []byte("\x89PNG\r\n\x1a\ntest")

func mapRequest(server *APIServer, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "http://127.0.0.1/api/v1/map-tiles/"+path, nil)
	r.RemoteAddr = "127.0.0.1:50000"
	// A caller cannot bypass the companion's persistent cache.
	r.Header.Set("Cache-Control", "no-cache")
	w := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(w, r)
	return w
}

func TestMapTilesIdentificationAndPersistentCache(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/2/1/1.png" {
			t.Errorf("unexpected URL %s", r.URL)
		}
		if !strings.HasPrefix(r.UserAgent(), "ZZZWallpaperCompanion/") || !strings.Contains(r.UserAgent(), "github.com/Garruvex") {
			t.Errorf("unidentified UA: %s", r.UserAgent())
		}
		if r.Header.Get("Referer") != "" || r.Header.Get("Cache-Control") != "" || r.Header.Get("Pragma") != "" {
			t.Error("spoofed referrer or bypassed cache")
		}
		w.Header().Set("Cache-Control", "max-age=604800")
		w.Header().Set("ETag", `"tile-v1"`)
		w.Write(testPNG)
	}))
	defer upstream.Close()
	server := testServer(t)
	server.mapTiles.baseURL = upstream.URL
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := mapRequest(server, "2/1/1")
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), testPNG) {
				t.Errorf("tile failed: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent cache miss downloaded %d times", calls.Load())
	}
	// Recreate the service as if the companion restarted.
	server.mapTiles = newMapTileService(filepath.Dir(server.mapTiles.dir))
	server.mapTiles.baseURL = upstream.URL
	// Router holds the original receiver, so test persistence through a fresh service directly.
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("z", "2")
	r.SetPathValue("x", "1")
	r.SetPathValue("y", "1")
	w := httptest.NewRecorder()
	server.mapTiles.serve(w, r)
	if w.Code != 200 || calls.Load() != 1 {
		t.Fatal("persistent cache missed after restart")
	}
	if !strings.HasPrefix(w.Header().Get("Cache-Control"), "private, max-age=") {
		t.Fatal("tile response cache headers were overwritten")
	}
}

func TestMapTilesConditionalRevalidation(t *testing.T) {
	server := testServer(t)
	path := filepath.Join(server.mapTiles.dir, "2", "1", "1.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	cached := cachedMapTile{Body: testPNG, Expires: time.Now().Add(-time.Hour), TTL: time.Hour, ETag: `"old"`, Modified: "Wed, 07 Oct 2026 00:00:00 GMT"}
	body, _ := json.Marshal(cached)
	os.WriteFile(path, body, 0o600)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != cached.ETag || r.Header.Get("If-Modified-Since") != cached.Modified {
			t.Error("missing validators")
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer upstream.Close()
	server.mapTiles.baseURL = upstream.URL
	w := mapRequest(server, "2/1/1")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), testPNG) {
		t.Fatalf("304 failed: %d", w.Code)
	}
	body, _ = os.ReadFile(path)
	json.Unmarshal(body, &cached)
	if time.Until(cached.Expires) < 59*time.Minute {
		t.Fatal("304 did not refresh cache lifetime")
	}
}

func TestMapTilesRejectCoordinatesAndBackOff(t *testing.T) {
	server := testServer(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(http.StatusForbidden) }))
	defer upstream.Close()
	server.mapTiles.baseURL = upstream.URL
	for _, path := range []string{"20/1/1", "2/4/1", "2/1/-1", "2/no/1"} {
		if w := mapRequest(server, path); w.Code != 400 {
			t.Errorf("accepted %s: %d", path, w.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid coordinates reached provider")
	}
	for _, path := range []string{"2/1/1", "2/1/2"} {
		if w := mapRequest(server, path); w.Code != 503 {
			t.Errorf("blocked response: %d", w.Code)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("continued requesting blocked provider")
	}
	if _, err := os.Stat(filepath.Join(server.mapTiles.dir, "2", "1", "1.json")); !os.IsNotExist(err) {
		t.Fatal("cached a blocked tile")
	}
}

func TestMapCacheHeaders(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		control, age string
		ttl          time.Duration
		persist      bool
	}{
		{"", "", 7 * 24 * time.Hour, true},
		{"max-age=120", "30", 90 * time.Second, true},
		{"max-age=120, no-store", "", 0, false},
		{"max-age=120, no-cache", "", 0, true},
	} {
		h := http.Header{}
		h.Set("Cache-Control", test.control)
		h.Set("Age", test.age)
		ttl, persist := mapTileTTL(h, now, 7*24*time.Hour)
		if ttl != test.ttl || persist != test.persist {
			t.Errorf("%s: got %s/%v", test.control, ttl, persist)
		}
	}
}

func TestMapTilesRejectInvalidBody(t *testing.T) {
	server := testServer(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("blocked page")) }))
	defer upstream.Close()
	server.mapTiles.baseURL = upstream.URL
	if w := mapRequest(server, "2/1/1"); w.Code != 502 {
		t.Fatalf("accepted HTML: %d", w.Code)
	}
}
