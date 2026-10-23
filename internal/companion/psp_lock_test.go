package companion

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Garruvex/zzz-wallpaper-companion/internal/gamepad"
)

func TestPSPStaysLockedUntilTheSecret(t *testing.T) {
	server := testServer(t)
	write(t, filepath.Join(server.romDir(), "Patapon", "game.iso"), "PSP")
	write(t, filepath.Join(server.romDir(), "Patapon", "save.zzzpsp"), "SAVE")
	write(t, filepath.Join(server.romDir(), "Sonic", "game.md"), "MD")
	write(t, filepath.Join(server.romDir(), "Loose.cso"), "PSP")

	var listed struct{ Carts []cart }
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/carts", nil).Body.Bytes(), &listed)
	// A locked game keeps its tape on the shelf, marked locked.
	if len(listed.Carts) != 2 || listed.Carts[0].ID != "Patapon" || !listed.Carts[0].Locked || listed.Carts[0].Tape != 1 || listed.Carts[1].ID != "Sonic" || listed.Carts[1].Locked || listed.Carts[1].Tape != 2 {
		t.Fatalf("the wallpaper's list: %+v", listed.Carts)
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
	// Its tape can be moved about the shelf like any other.
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/carts/Patapon/do/tape?n=5", nil); r.Code != http.StatusNoContent {
		t.Fatalf("locked PSP tape not moved: %d", r.Code)
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

	if err := server.setPSPUnlocked(true); err != nil {
		t.Fatal(err)
	}
	state = server.pspState()
	if !state.Unlocked || len(state.Locked) != 0 {
		t.Fatalf("unlock %+v", state)
	}
	listed = struct{ Carts []cart }{}
	decode(t, cartRequest(t, server, http.MethodGet, "/api/v1/carts", nil).Body.Bytes(), &listed)
	if len(listed.Carts) != 3 || listed.Carts[1].ID != "Patapon" || listed.Carts[1].Locked || listed.Carts[1].Tape != 2 {
		t.Fatalf("unlocked list %+v", listed.Carts)
	}
	if r := cartRequest(t, server, http.MethodPost, "/api/v1/games/current", []byte(`{"id":"Patapon"}`)); r.Code != http.StatusOK {
		t.Fatalf("unlocked PSP game not selected: %d %s", r.Code, r.Body)
	}

	// Locking again takes the PSP game off the TV.
	if err := server.setPSPUnlocked(false); err != nil {
		t.Fatal(err)
	}
	state = server.pspState()
	if state.Unlocked || server.currentGame().ID != "" || !server.currentGame().Fresh {
		t.Fatalf("relock %+v %+v", state, server.currentGame())
	}
}

func TestPSPCodeOnAControllerWithAnEmptyDeck(t *testing.T) {
	server := testServer(t)
	clock := time.Unix(1000, 0)
	server.pspCode.now = func() time.Time { return clock }
	enter := func(names ...string) {
		for _, name := range names {
			bit := -1
			for _, button := range pspCodeButtons {
				if button.name == name {
					bit = button.bit
				}
			}
			if name == "start" {
				bit = gamepad.ButtonStart
			}
			clock = clock.Add(200 * time.Millisecond)
			server.watchPSPCode(1 << bit)
			clock = clock.Add(100 * time.Millisecond)
			server.watchPSPCode(0)
		}
	}
	code := []string{"up", "up", "down", "down", "left", "right", "left", "right", "b", "a"}

	enter(code[:5]...)
	clock = clock.Add(2 * time.Second) // too long a pause starts over
	enter(code[5:]...)
	if server.pspUnlocked() {
		t.Fatal("unlocked across a pause")
	}
	enter(append([]string{"start"}, code...)...) // a stray press first doesn't matter
	if !server.pspUnlocked() {
		t.Fatal("the code did not unlock PSP")
	}
	updates, cancel := server.input.Subscribe()
	defer cancel()
	if snapshot := <-updates; !snapshot.PSPUnlocked {
		t.Fatalf("the stream does not say PSP is unlocked: %+v", snapshot)
	}

	// With a tape in the deck the code does nothing.
	write(t, filepath.Join(server.romDir(), "Sonic", "game.md"), "MD")
	server.selectGame("Sonic", false)
	enter(code...)
	if !server.pspUnlocked() {
		t.Fatal("locked with a tape in the deck")
	}
	server.selectGame("", false)
	enter(code...)
	if server.pspUnlocked() {
		t.Fatal("the code did not lock PSP again")
	}
}

// The unlock lasts until the companion quits; the next start begins locked, even if an older build
// left its marker file.
func TestPSPLocksAgainWhenTheCompanionRestarts(t *testing.T) {
	server := testServer(t)
	if err := server.setPSPUnlocked(true); err != nil || !server.pspUnlocked() {
		t.Fatal("not unlocked")
	}
	write(t, filepath.Join(server.dataDir, pspUnlockFile), "old marker")
	restarted := newAPIServer(server.config, server.resolver, server.ffmpeg)
	t.Cleanup(restarted.systemStats.Close)
	if restarted.pspUnlocked() {
		t.Fatal("a restart kept PSP unlocked")
	}
	if _, err := os.Stat(filepath.Join(server.dataDir, pspUnlockFile)); !os.IsNotExist(err) {
		t.Fatal("the old marker file was left behind")
	}
}
