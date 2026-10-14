package companion

import (
	"bytes"
	"encoding/json"
	"github.com/Garruvex/zzz-wallpaper-companion/api"
	"github.com/Garruvex/zzz-wallpaper-companion/internal/gamepad"
	"github.com/Garruvex/zzz-wallpaper-companion/internal/lyrics"
	"github.com/Garruvex/zzz-wallpaper-companion/internal/systemstats"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineDocsUseExistingAccessPolicy(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/docs", "/docs/index.html", "/docs/explorer.js", "/docs/swagger-ui-bundle.js", "/docs/swagger-ui.css", "/openapi.json"} {
		t.Run(path, func(t *testing.T) {
			for _, tc := range []struct {
				remote, origin string
				status         int
			}{
				{"127.0.0.1:3210", "http://127.0.0.1:9123", 200},
				{"127.0.0.1:3210", "null", 200},
				{"192.0.2.1:3210", "", 403},
				{"127.0.0.1:3210", "https://example.com", 403},
			} {
				r := httptest.NewRequest("GET", "http://127.0.0.1:9123"+path, nil)
				r.RemoteAddr = tc.remote
				r.Header.Set("Origin", tc.origin)
				w := httptest.NewRecorder()
				s.http.Handler.ServeHTTP(w, r)
				if w.Code != tc.status {
					t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
				}
				if tc.status == 200 && path == "/openapi.json" && !bytes.Equal(w.Body.Bytes(), api.Contract) {
					t.Fatal("served contract drift")
				}
			}
		})
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:9123/docs/missing.js", nil)
	r.RemoteAddr = "127.0.0.1:3210"
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:9123/docs", nil)
	r.RemoteAddr = "127.0.0.1:3210"
	w = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), "https://") || strings.Contains(w.Body.String(), "8765") {
		t.Fatal("docs depends on remote assets or hard-coded port")
	}
	r = httptest.NewRequest("GET", "http://127.0.0.1:9123/settings", nil)
	r.RemoteAddr = "127.0.0.1:3210"
	w = httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "href=\"/docs\"") {
		t.Fatal("missing explorer link")
	}
}

func TestRecoverySettingsHaveNoExplorerLink(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	s.renderSettings(w, "port unavailable")
	if strings.Contains(w.Body.String(), "href=\"/docs\"") {
		t.Fatal("recovery listener cannot serve docs")
	}
}

func TestDocumentedTopLevelWireShapes(t *testing.T) {
	// Field presence and omissions only, not runtime schema validation.
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage
				Required   []string
			}
		}
	}
	if err := json.Unmarshal(api.Contract, &doc); err != nil {
		t.Fatal(err)
	}
	values := map[string]any{
		"lyrics.Request":       lyrics.LyricsRequest{},
		"lyrics.Response":      lyrics.LyricsResponse{},
		"lyrics.Line":          lyrics.LyricLine{},
		"system.Snapshot":      systemstats.SystemStatsSnapshot{},
		"youtube.Media":        resolvedMedia{},
		"youtube.PlaylistItem": playlistItem{},
		"input.Snapshot":       gamepad.Snapshot{},
		"dates.Document":       DateDocument{Version: 1, Events: []ImportantDate{}},
		"dates.Event":          ImportantDate{},
		"holidays.Settings":    HolidaySettings{},
		"holidays.Record":      HolidayRecord{},
		"settings.Document":    defaultSettings(),
		"games.Cart":           cart{},
		"games.Selection":      gameSelection{},
	}
	for name, value := range values {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		schema, ok := doc.Components.Schemas[name]
		if !ok {
			t.Fatal(name)
		}
		for field := range fields {
			if _, ok := schema.Properties[field]; !ok {
				t.Errorf("%s undocumented field %s", name, field)
			}
		}
		for _, field := range schema.Required {
			if _, ok := fields[field]; !ok {
				t.Errorf("%s required field absent %s", name, field)
			}
		}
	}
}

func TestDocsBrowser(t *testing.T) {
	if os.Getenv("COMPANION_DOCS_BROWSER") != "1" {
		t.Skip("set COMPANION_DOCS_BROWSER=1 with Playwright/browser paths")
	}
	s := testServer(t)
	host := httptest.NewServer(s.http.Handler)
	defer host.Close()
	node := os.Getenv("DOCS_NODE")
	if node == "" {
		node = "node"
	}
	cmd := exec.Command(node, filepath.Join("..", "..", "tools", "docs-browser.cjs"), host.URL)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, output)
	}
	t.Log(string(output))
}
