package companion

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renameRequest(t *testing.T, server *APIServer, id, title string) int {
	t.Helper()
	body := `{"id":` + quote(id) + `,"title":` + quote(title) + `}`
	return cartRequest(t, server, http.MethodPost, "/games/rename", []byte(body)).Code
}

func quote(s string) string {
	out, _ := json.Marshal(s)
	return string(out)
}

func titleOf(t *testing.T, server *APIServer, id string) string {
	t.Helper()
	c, ok := server.cartByID(id)
	if !ok {
		t.Fatalf("no game %s", id)
	}
	return c.Title
}

func TestRenamingAGameChangesOnlyItsTitle(t *testing.T) {
	server := testServer(t)
	write(t, filepath.Join(server.romDir(), "pokemon copy", "game.gba"), "GBA ROM")
	write(t, filepath.Join(server.romDir(), "pokemon copy", "save.sav"), "SRAM")
	write(t, filepath.Join(server.romDir(), "pokemon copy", "cart.json"), `{"sha1":"abc","rev":4,"tape":3,"shelf":true}`)

	if code := renameRequest(t, server, "pokemon copy", "  Pokemon   Emerald "); code != http.StatusOK {
		t.Fatalf("rename answered %d", code)
	}
	c, _ := server.cartByID("pokemon copy")
	if c.ID != "pokemon copy" || c.Title != "Pokemon Emerald" || c.Rev != 4 {
		t.Fatalf("after rename: %+v", c)
	}
	meta := cartMeta(c.dir)
	if meta["sha1"] != "abc" || meta["tape"] != float64(3) || meta["shelf"] != true || meta["rev"] != float64(4) {
		t.Fatalf("the rename changed more than the title: %v", meta)
	}
	if data, _ := os.ReadFile(filepath.Join(server.romDir(), "pokemon copy", "save.sav")); string(data) != "SRAM" {
		t.Fatalf("the save changed: %q", data)
	}

	// A name in any language, and a name that is too long gets cut.
	renameRequest(t, server, "pokemon copy", "宝可梦 绿宝石")
	if got := titleOf(t, server, "pokemon copy"); got != "宝可梦 绿宝石" {
		t.Fatalf("title %q", got)
	}
	renameRequest(t, server, "pokemon copy", strings.Repeat("あ", 60))
	if got := []rune(titleOf(t, server, "pokemon copy")); len(got) != maxTitleRunes {
		t.Fatalf("a long title kept %d characters", len(got))
	}

	// An empty name goes back to the file's name.
	renameRequest(t, server, "pokemon copy", "   ")
	if got := titleOf(t, server, "pokemon copy"); got != "pokemon copy" {
		t.Fatalf("title %q", got)
	}
}

func TestRenamingRefusesUnknownGamesAndBadRequests(t *testing.T) {
	server := testServer(t)
	write(t, filepath.Join(server.romDir(), "Sonic", "game.md"), "MD")
	for _, id := range []string{"Nope", "../Sonic", ".removed", ""} {
		if code := renameRequest(t, server, id, "x"); code != http.StatusNotFound {
			t.Fatalf("rename of %q answered %d", id, code)
		}
	}
	if r := cartRequest(t, server, http.MethodPost, "/games/rename", []byte(`{"id":"Sonic","title":"x","extra":1}`)); r.Code != http.StatusBadRequest {
		t.Fatalf("unknown field answered %d", r.Code)
	}
	if got := titleOf(t, server, "Sonic"); got != "Sonic" {
		t.Fatalf("title changed to %q", got)
	}
}
