package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decode(t *testing.T, body []byte, value any) {
	t.Helper()
	if err := json.Unmarshal(body, value); err != nil {
		t.Fatalf("%s: %v", body, err)
	}
}

func TestAddingAGameFindsTheSameROMUnderAnotherName(t *testing.T) {
	server := testServer(t)
	r := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=Emerald.gba", []byte("GBA ROM"))
	if r.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", r.Code, r.Body)
	}
	var added struct {
		Cart     cart
		Existing bool
	}
	decode(t, r.Body.Bytes(), &added)
	if added.Cart.ID != "Emerald" || added.Cart.System != "gba" || added.Cart.SHA1 == "" {
		t.Fatalf("added %+v", added.Cart)
	}
	write(t, filepath.Join(server.romDir(), "Emerald", "save.sav"), "SRAM")

	r = cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=pokemon%20copy.gba", []byte("GBA ROM"))
	decode(t, r.Body.Bytes(), &added)
	if r.Code != http.StatusOK || !added.Existing || added.Cart.ID != "Emerald" {
		t.Fatalf("same ROM: %d %+v", r.Code, added)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=readme.txt", []byte("x")); r.Code != http.StatusBadRequest {
		t.Fatalf("non-game accepted: %d", r.Code)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=..%2F..%2Fevil.gba", []byte("other")); r.Code != http.StatusCreated {
		t.Fatalf("path in name: %d", r.Code)
	}
	if _, err := os.Stat(filepath.Join(server.romDir(), "evil", "game.gba")); err != nil {
		t.Fatalf("name was not reduced to a folder name: %v", err)
	}
}

func TestSelectingAGameForTheTV(t *testing.T) {
	server := testServer(t)
	write(t, filepath.Join(server.romDir(), "Sonic", "game.md"), "MD")
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/games/current", []byte(`{"id":"Nope"}`)); r.Code != http.StatusNotFound {
		t.Fatalf("unknown game selected: %d", r.Code)
	}
	cartRequest(t, server, http.MethodPost, "/api/v1/games/current", []byte(`{"id":"Sonic"}`))
	var got gameSelection
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/games/current", nil).Body.Bytes(), &got)
	if got.ID != "Sonic" || got.Seq != 1 || got.Fresh {
		t.Fatalf("selection %+v", got)
	}
}

func TestResetAndImportShelveSavesAndStopOldWrites(t *testing.T) {
	server := testServer(t)
	dir := filepath.Join(server.romDir(), "Emerald")
	write(t, filepath.Join(dir, "game.gba"), "GBA")
	write(t, filepath.Join(dir, "save.sav"), "OLD SAVE")
	write(t, filepath.Join(dir, "state.state"), "STATE")
	write(t, filepath.Join(dir, "state-1.state"), "OLDER")
	cartRequest(t, server, http.MethodPost, "/api/v1/games/current", []byte(`{"id":"Emerald"}`))

	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts/Emerald/do/reset", nil); r.Code != http.StatusOK {
		t.Fatalf("reset: %d %s", r.Code, r.Body)
	}
	for _, name := range []string{"save.sav", "state.state", "state-1.state"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still there after reset", name)
		}
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "backups", "*", "save.sav"))
	if len(backups) != 1 || read(t, backups[0]) != "OLD SAVE" {
		t.Fatalf("save not kept in backups: %v", backups)
	}
	var sel gameSelection
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/games/current", nil).Body.Bytes(), &sel)
	if !sel.Fresh || sel.Seq != 2 {
		t.Fatalf("TV not told to reload fresh: %+v", sel)
	}

	// The game still running from before the reset can't save over it.
	if code := revisionRequest(t, server, "0", "STALE"); code != http.StatusConflict {
		t.Fatalf("stale save accepted: %d", code)
	}
	if code := revisionRequest(t, server, "1", "NEW"); code != http.StatusNoContent {
		t.Fatalf("current save rejected: %d", code)
	}

	r := cartRequest(t, server, http.MethodPost, "/api/v1/carts/Emerald/do/import", []byte("IMPORTED"))
	if r.Code != http.StatusOK {
		t.Fatalf("import: %d %s", r.Code, r.Body)
	}
	if read(t, filepath.Join(dir, "save.sav")) != "IMPORTED" {
		t.Fatal("import did not replace the save")
	}
	if c, _ := readCart(server.romDir(), "Emerald"); c.Rev != 2 {
		t.Fatalf("rev %d after reset and import", c.Rev)
	}
}

func revisionRequest(t *testing.T, server *APIServer, rev, body string) int {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/carts/Emerald/save", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Cart-Rev", rev)
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestRestoringAnEarlierResumePoint(t *testing.T) {
	server := testServer(t)
	dir := filepath.Join(server.romDir(), "Emerald")
	write(t, filepath.Join(dir, "game.gba"), "GBA")
	write(t, filepath.Join(dir, "state.state"), "NOW")
	write(t, filepath.Join(dir, "state-2.state"), "YESTERDAY")
	var info struct{ States []struct{ N int } }
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/carts/Emerald/details", nil).Body.Bytes(), &info)
	if len(info.States) != 1 || info.States[0].N != 2 {
		t.Fatalf("details %+v", info)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts/Emerald/do/restore?state=2", nil); r.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", r.Code, r.Body)
	}
	if read(t, filepath.Join(dir, "state.state")) != "YESTERDAY" {
		t.Fatal("resume point not restored")
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts/Emerald/do/restore?state=3", nil); r.Code != http.StatusNotFound {
		t.Fatalf("missing resume point: %d", r.Code)
	}
}

func TestRemovingAGameMovesItAside(t *testing.T) {
	server := testServer(t)
	write(t, filepath.Join(server.romDir(), "Sonic", "game.md"), "MD")
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts/Sonic/do/remove", nil); r.Code != http.StatusNoContent {
		t.Fatalf("remove: %d", r.Code)
	}
	moved, _ := filepath.Glob(filepath.Join(server.romDir(), ".removed", "Sonic *", "game.md"))
	if len(moved) != 1 {
		t.Fatal("game not moved to .removed")
	}
	if strings.Contains(cartRequest(t, server, http.MethodGet, "/api/v1/carts", nil).Body.String(), "Sonic") {
		t.Fatal("removed game still listed")
	}
	if r := cartRequest(t, server, http.MethodGet, "/games", nil); r.Code != http.StatusOK || !strings.Contains(r.Body.String(), "Play on TV") {
		t.Fatalf("games page: %d", r.Code)
	}
}
