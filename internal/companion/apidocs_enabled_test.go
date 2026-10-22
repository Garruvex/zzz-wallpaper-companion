//go:build apidocs

package companion

import (
	"bytes"
	"github.com/Garruvex/zzz-wallpaper-companion/api"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineDocsUseExistingAccessPolicy(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/ui/api.js", "/docs", "/docs/index.html", "/docs/explorer.js", "/docs/swagger-ui-bundle.js", "/docs/swagger-ui.css", "/openapi.json"} {
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
