package companion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contract "github.com/Garruvex/zzz-wallpaper-companion/api"
)

func TestBrowserPagesUseContractHelper(t *testing.T) {
	digest := fmt.Sprintf("%x", sha256.Sum256(bytes.ReplaceAll(contract.Contract, []byte("\r\n"), []byte("\n"))))
	if !strings.Contains(operationsJS, `const CONTRACT_DIGEST = "`+digest+`";`) {
		t.Fatal("embedded browser contract digest drift")
	}
	s := testServer(t)
	for _, path := range []string{"/settings", "/dates", "/holidays", "/games"} {
		response := cartRequest(t, s, http.MethodGet, path, nil)
		body := response.Body.String()
		if !strings.Contains(body, `src="/ui/api.js"`) || !strings.Contains(body, "CompanionAPI.request") || strings.Contains(body, "fetch(") || strings.Contains(body, "'/api/") || strings.Contains(body, "`/api/") {
			t.Fatalf("page bypasses generated browser helper: %s", path)
		}
	}
	response := cartRequest(t, s, http.MethodGet, "/ui/api.js", nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "CompanionContract") || !strings.Contains(response.Body.String(), "CompanionAPI") {
		t.Fatal("missing embedded browser helper")
	}
}

func TestSettingsBrowser(t *testing.T) {
	if os.Getenv("COMPANION_DOCS_BROWSER") != "1" {
		t.Skip("enable browser integration with Playwright/browser paths")
	}
	s := testServer(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"countryCode":"US","name":"United States"}]`))
	}))
	defer provider.Close()
	s.holidays.baseURL = provider.URL
	host := httptest.NewServer(s.http.Handler)
	defer host.Close()
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	recovery := testServer(t)
	settings := recovery.config.Get()
	settings.Port = occupied.Addr().(*net.TCPAddr).Port
	if err := recovery.config.Save(settings); err != nil {
		t.Fatal(err)
	}
	listener, _, err := recovery.prepareListener()
	if err != nil {
		t.Fatal(err)
	}
	go recovery.http.Serve(listener)
	defer recovery.Shutdown(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	node := os.Getenv("DOCS_NODE")
	if node == "" {
		node = "node"
	}
	cmd := exec.CommandContext(ctx, node, filepath.Join("..", "..", "tools", "settings-browser.cjs"), host.URL, "http://"+listener.Addr().String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("settings browser: %v\n%s", err, output)
	}
	t.Log(string(output))
}
