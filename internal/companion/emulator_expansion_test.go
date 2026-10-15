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
		{"Patapon.iso", "psp", "save.zzzpsp"}, {"Portable.cso", "psp", "save.zzzpsp"},
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
	response := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=Empty.iso", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatal(response.Code)
	}
	entries, err := os.ReadDir(server.romDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("empty upload left files: %v %v", entries, err)
	}
}
