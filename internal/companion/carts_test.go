package companion

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func cartRequest(t *testing.T, server *APIServer, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1"+target, bytes.NewReader(body))
	request.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	return recorder
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(data)
}

func TestLooseROMsBecomeCartridgesWithTheirSaves(t *testing.T) {
	server := testServer(t)
	root := server.ensureROMDir()
	write(t, filepath.Join(root, "Emerald.gba"), "GBA")
	write(t, filepath.Join(root, "Emerald.sav"), "SRAM")
	write(t, filepath.Join(root, "Emerald.state"), "STATE")
	write(t, filepath.Join(root, "Mario World.sfc"), "SNES")
	write(t, filepath.Join(root, "Sonic", "game.md"), "MD")
	write(t, filepath.Join(root, "Sonic", "cart.json"), `{"title":"Sonic 2","color":"#2050ff"}`)
	write(t, filepath.Join(root, "Sonic", "label.png"), "PNG")
	write(t, filepath.Join(root, "notes.txt"), "not a game")
	write(t, filepath.Join(root, "Empty", "readme.txt"), "no rom")

	r := cartRequest(t, server, http.MethodGet, "/api/v1/carts", nil)
	var got struct{ Carts []cart }
	if err := json.Unmarshal(r.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []cart{
		{ID: "Emerald", Title: "Emerald", System: "gba", ROM: "game.gba", OnShelf: true},
		{ID: "Mario World", Title: "Mario World", System: "snes", ROM: "game.sfc", OnShelf: true},
		{ID: "Sonic", Title: "Sonic 2", System: "segaMD", ROM: "game.md", Color: "#2050ff", Label: true, OnShelf: true},
	}
	if len(got.Carts) != len(want) {
		t.Fatalf("carts = %+v", got.Carts)
	}
	for i := range want {
		if got.Carts[i] != want[i] {
			t.Fatalf("cart %d = %+v, want %+v", i, got.Carts[i], want[i])
		}
	}
	if read(t, filepath.Join(root, "Emerald", "save.sav")) != "SRAM" || read(t, filepath.Join(root, "Emerald", "state.state")) != "STATE" {
		t.Fatal("old saves should move into the cartridge")
	}
	if _, err := os.Stat(filepath.Join(root, "notes.txt")); err != nil {
		t.Fatal("non-ROM files must be left alone")
	}
}

func TestCartridgeFilesReadAndWrite(t *testing.T) {
	server := testServer(t)
	root := server.ensureROMDir()
	write(t, filepath.Join(root, "Mario World", "game.sfc"), "SNES")
	cart := "/api/v1/carts/" + url.PathEscape("Mario World")

	if r := cartRequest(t, server, http.MethodGet, cart+"/rom", nil); r.Body.String() != "SNES" {
		t.Fatalf("rom = %d %q", r.Code, r.Body.String())
	}
	if r := cartRequest(t, server, http.MethodGet, cart+"/save", nil); r.Code != http.StatusNotFound {
		t.Fatalf("missing save: %d", r.Code)
	}
	cartRequest(t, server, http.MethodPost, cart+"/save", []byte("SRAM1"))
	cartRequest(t, server, http.MethodPost, cart+"/save", []byte("SRAM2"))
	if read(t, filepath.Join(root, "Mario World", "save.srm")) != "SRAM2" || read(t, filepath.Join(root, "Mario World", "save.srm.bak")) != "SRAM1" {
		t.Fatal("SNES saves should be save.srm with a .bak of the previous one")
	}

	cartRequest(t, server, http.MethodPost, cart+"/state", []byte("S1"))
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(filepath.Join(root, "Mario World", "state.state"), old, old)
	cartRequest(t, server, http.MethodPost, cart+"/state", []byte("S2"))
	cartRequest(t, server, http.MethodPost, cart+"/state", []byte("S3"))
	if read(t, filepath.Join(root, "Mario World", "state.state")) != "S3" || read(t, filepath.Join(root, "Mario World", "state-1.state")) != "S1" {
		t.Fatal("an old state should be kept as state-1 before a new one replaces it")
	}

	for _, bad := range []string{cart + "/rom", cart + "/label", "/api/v1/carts/..%2Fx/save", "/api/v1/carts/Missing/save"} {
		if r := cartRequest(t, server, http.MethodPost, bad, []byte("x")); r.Code == http.StatusNoContent {
			t.Fatalf("%s accepted a write", bad)
		}
	}
	for _, bad := range []string{cart + "/other", "/api/v1/carts/..%2F..%2Fsettings.json/rom", "/api/v1/carts/.hidden/rom"} {
		if r := cartRequest(t, server, http.MethodGet, bad, nil); r.Code == http.StatusOK {
			t.Fatalf("%s was served", bad)
		}
	}
}
