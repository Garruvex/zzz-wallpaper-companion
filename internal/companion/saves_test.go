package companion

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestSavesLiveNextToTheROM(t *testing.T) {
	server := testServer(t)
	dir := server.ensureROMDir()
	if err := os.WriteFile(filepath.Join(dir, "My Game.gba"), []byte("ROM"), 0o600); err != nil {
		t.Fatal(err)
	}
	do := func(method, target string, body []byte) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "http://127.0.0.1"+target, bytes.NewReader(body))
		request.RemoteAddr = "127.0.0.1:1234"
		recorder := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(recorder, request)
		return recorder
	}
	game := "/api/v1/roms/" + url.PathEscape("My Game.gba")
	if r := do(http.MethodGet, game+"/save", nil); r.Code != http.StatusNotFound {
		t.Fatalf("missing save: %d", r.Code)
	}
	if r := do(http.MethodPost, game+"/save", []byte("SRAM1")); r.Code != http.StatusNoContent {
		t.Fatalf("write: %d %s", r.Code, r.Body)
	}
	do(http.MethodPost, game+"/save", []byte("SRAM2"))
	if r := do(http.MethodGet, game+"/save", nil); r.Body.String() != "SRAM2" {
		t.Fatalf("read back %q", r.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "My Game.sav")); string(data) != "SRAM2" {
		t.Fatalf("My Game.sav = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "My Game.sav.bak")); string(data) != "SRAM1" {
		t.Fatalf("backup = %q", data)
	}
	do(http.MethodPost, game+"/state", []byte("STATE"))
	if data, _ := os.ReadFile(filepath.Join(dir, "My Game.state")); string(data) != "STATE" {
		t.Fatalf("My Game.state = %q", data)
	}
	for _, bad := range []string{game + "/other", "/api/v1/roms/missing.gba/save", "/api/v1/roms/x.txt/save", "/api/v1/roms/..%2Fx.gba/save"} {
		if r := do(http.MethodPost, bad, []byte("x")); r.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", bad, r.Code)
		}
	}
	if r := do(http.MethodPost, game+"/save", nil); r.Code != http.StatusBadRequest {
		t.Fatalf("empty save accepted: %d", r.Code)
	}
}
