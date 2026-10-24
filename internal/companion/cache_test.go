package companion

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestClearingTheCacheEmptiesOnlyRemovedGamesAndMapTiles(t *testing.T) {
	server := testServer(t)
	write(t, filepath.Join(server.romDir(), "Emerald", "game.gba"), "GBA ROM")
	write(t, filepath.Join(server.romDir(), "Emerald", "save.sav"), "SRAM")
	write(t, filepath.Join(server.romDir(), "Emerald", "backups", "old.sav"), "OLD")
	write(t, filepath.Join(server.romDir(), ".removed", "Sonic 2026-10-24 101010", "game.md"), "12345678")
	write(t, filepath.Join(server.romDir(), ".removed", "Sonic 2026-10-24 101010", "save.srm"), "SAVE")
	write(t, filepath.Join(server.dataDir, "map-tiles", "3", "4", "5.json"), "tile")

	var use cacheUse
	decode(t, cartRequest(t, server, http.MethodGet, "/games/cache", nil).Body.Bytes(), &use)
	if use.RemovedGames != 1 || use.RemovedBytes != 12 || use.MapBytes != 4 {
		t.Fatalf("use before: %+v", use)
	}

	if r := cartRequest(t, server, http.MethodPost, "/games/cache/clear", []byte(`{}`)); r.Code != http.StatusBadRequest {
		t.Fatalf("an unconfirmed clear answered %d", r.Code)
	}
	if _, err := os.Stat(filepath.Join(server.romDir(), ".removed")); err != nil {
		t.Fatalf("an unconfirmed clear deleted the removed games: %v", err)
	}

	if r := cartRequest(t, server, http.MethodPost, "/games/cache/clear", []byte(`{"confirm":true}`)); r.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", r.Code, r.Body)
	}
	for _, gone := range []string{filepath.Join(server.romDir(), ".removed"), filepath.Join(server.dataDir, "map-tiles")} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", gone)
		}
	}
	for _, kept := range []string{"game.gba", "save.sav", filepath.Join("backups", "old.sav")} {
		if _, err := os.Stat(filepath.Join(server.romDir(), "Emerald", kept)); err != nil {
			t.Fatalf("a game's own file was deleted: %v", err)
		}
	}
	decode(t, cartRequest(t, server, http.MethodGet, "/games/cache", nil).Body.Bytes(), &use)
	if use != (cacheUse{}) {
		t.Fatalf("use after: %+v", use)
	}
}
