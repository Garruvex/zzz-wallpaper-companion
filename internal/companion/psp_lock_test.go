package companion

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestPSPStaysLockedUntilTheSecret(t *testing.T) {
	server := testServer(t)
	write(t, filepath.Join(server.romDir(), "Patapon", "game.iso"), "PSP")
	write(t, filepath.Join(server.romDir(), "Patapon", "save.zzzpsp"), "SAVE")
	write(t, filepath.Join(server.romDir(), "Sonic", "game.md"), "MD")
	write(t, filepath.Join(server.romDir(), "Loose.cso"), "PSP")

	var listed struct{ Carts []cart }
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/carts", nil).Body.Bytes(), &listed)
	if len(listed.Carts) != 1 || listed.Carts[0].ID != "Sonic" {
		t.Fatalf("the wallpaper sees a locked PSP game: %+v", listed.Carts)
	}
	if _, err := os.Stat(filepath.Join(server.romDir(), "Loose.cso")); err != nil {
		t.Fatalf("a loose PSP file was imported while locked: %v", err)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts?name=New.iso", []byte("PSP2")); r.Code != http.StatusBadRequest {
		t.Fatalf("PSP upload accepted while locked: %d", r.Code)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/games/current", []byte(`{"id":"Patapon"}`)); r.Code != http.StatusConflict {
		t.Fatalf("locked PSP game selected: %d", r.Code)
	}
	if r := cartRequest(t, server, http.MethodGet, "/api/v1/carts/Patapon/rom", nil); r.Code != http.StatusNotFound {
		t.Fatalf("locked PSP ROM served: %d", r.Code)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts/Patapon/do/shelf?on=1", nil); r.Code != http.StatusConflict {
		t.Fatalf("locked PSP game shelved: %d", r.Code)
	}
	// Its saves are still the player's to download.
	if r := cartRequest(t, server, http.MethodGet, "/api/v1/carts/Patapon/save", nil); r.Code != http.StatusOK || r.Body.String() != "SAVE" {
		t.Fatalf("locked PSP save: %d %s", r.Code, r.Body)
	}
	var state pspLockState
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/games/psp", nil).Body.Bytes(), &state)
	if state.Unlocked || len(state.Locked) != 1 || state.Locked[0].ID != "Patapon" {
		t.Fatalf("lock state %+v", state)
	}

	decode(t, cartRequest(t, server, http.MethodPost, "/api/v1/games/psp", []byte(`{"unlocked":true}`)).Body.Bytes(), &state)
	if !state.Unlocked || len(state.Locked) != 0 {
		t.Fatalf("unlock %+v", state)
	}
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/carts", nil).Body.Bytes(), &listed)
	if len(listed.Carts) != 3 {
		t.Fatalf("unlocked list %+v", listed.Carts)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/games/current", []byte(`{"id":"Patapon"}`)); r.Code != http.StatusOK {
		t.Fatalf("unlocked PSP game not selected: %d %s", r.Code, r.Body)
	}

	// Locking again takes the PSP game off the TV.
	decode(t, cartRequest(t, server, http.MethodPost, "/api/v1/games/psp", []byte(`{"unlocked":false}`)).Body.Bytes(), &state)
	if state.Unlocked || server.currentGame().ID != "" || !server.currentGame().Fresh {
		t.Fatalf("relock %+v %+v", state, server.currentGame())
	}
}
