package companion

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestExperimentalDiscImportsAndSaves(t *testing.T) {
	server := testServer(t)
	for _, sample := range []struct{ name, system, save string }{
		{"Raiden.chd", "psx", "save.srm"}, {"PS1.pbp", "psx", "save.srm"},
	} {
		response := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name="+sample.name, []byte(sample.name))
		if response.Code != http.StatusCreated {
			t.Fatalf("%s: %d %s", sample.name, response.Code, response.Body)
		}
		var result struct{ Cart cart }
		decode(t, response.Body.Bytes(), &result)
		c, ok := server.cartByID(result.Cart.ID)
		if !ok || c.System != sample.system || c.save != sample.save {
			t.Fatalf("unexpected cart: %+v", c)
		}
		data := []byte("opaque save bytes")
		response = cartRequest(t, server, http.MethodPost, "/api/v1/carts/"+c.ID+"/save", data)
		if response.Code != http.StatusNoContent {
			t.Fatalf("save: %d %s", response.Code, response.Body)
		}
		if got := read(t, filepath.Join(c.dir, sample.save)); got != string(data) {
			t.Fatal(got)
		}
	}
	uploads, err := filepath.Glob(filepath.Join(server.romDir(), ".upload-*"))
	if err != nil || len(uploads) != 0 {
		t.Fatalf("upload temporaries leaked: %v %v", uploads, err)
	}
}

func TestDiscLimitsAndUploadCleanup(t *testing.T) {
	if romByteLimit("gba") != 64<<20 || romByteLimit("psp") != 2<<30 || romByteLimit("psx") != 2<<30 {
		t.Fatal("incorrect ROM limits")
	}
	if cartByteLimit(cart{System: "psp"}, "save") != 64<<20 || cartByteLimit(cart{System: "psp"}, "state") != 256<<20 || cartByteLimit(cart{System: "gba"}, "save") != 8<<20 {
		t.Fatal("incorrect save limits")
	}
	server := testServer(t)
	response := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=Empty.chd", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatal(response.Code)
	}
	entries, err := os.ReadDir(server.romDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty upload left files: %v %v", entries, err)
	}
}

func TestRetiredPSPPreservesFilesButCannotImportOrPlay(t *testing.T) {
	server := testServer(t)
	dir := filepath.Join(server.ensureROMDir(), "OldPSP")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game.iso"), []byte("original game"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "save.zzzpsp"), []byte("original save"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cart.json"), []byte(`{"shelf":true}`), 0644); err != nil {
		t.Fatal(err)
	}
	carts := server.cartList()
	if len(carts) != 1 || carts[0].System != "psp" || carts[0].OnShelf {
		t.Fatalf("retired cart: %+v", carts)
	}
	for _, extension := range []string{"iso", "cso", "elf"} {
		response := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=New."+extension, []byte("new"))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("upload %s: %d", extension, response.Code)
		}
	}
	response := cartRequest(t, server, http.MethodPost, "/api/v1/games/current", []byte(`{"id":"OldPSP"}`))
	if response.Code != http.StatusConflict {
		t.Fatalf("play: %d %s", response.Code, response.Body)
	}
	response = cartRequest(t, server, http.MethodPost, "/api/v1/carts/OldPSP/do/shelf?on=1", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("shelf: %d %s", response.Code, response.Body)
	}
	response = cartRequest(t, server, http.MethodGet, "/api/v1/carts/OldPSP/save", nil)
	if response.Code != http.StatusOK || response.Body.String() != "original save" {
		t.Fatalf("save: %d %s", response.Code, response.Body)
	}
	if got := read(t, filepath.Join(dir, "game.iso")); got != "original game" {
		t.Fatal(got)
	}
}
