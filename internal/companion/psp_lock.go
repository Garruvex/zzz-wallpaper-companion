package companion

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// See games.psp.get and games.psp.save in api/openapi.json.

// PSP is not stable yet, so it stays locked until the player types the
// secret sequence on the Games page. While it is locked the companion takes
// no PSP files and never hands a PSP game to the wallpaper: the cartridge
// list leaves them out, and they can't be played, put on the shelf or have
// their ROM read. PSP games already in the library keep their folders and
// saves, and the Games page shows them greyed out. The same sequence locks it
// again.
//
// The unlock is a marker file next to the settings, so it survives restarts
// and updates without being part of the settings the settings page replaces.

const pspUnlockFile = "psp-unlocked"

func (s *APIServer) pspUnlocked() bool {
	_, err := os.Stat(filepath.Join(s.dataDir, pspUnlockFile))
	return err == nil
}

func pspLocked(c cart, unlocked bool) bool { return c.System == "psp" && !unlocked }

func (s *APIServer) setPSPUnlocked(on bool) error {
	path := filepath.Join(s.dataDir, pspUnlockFile)
	if on {
		return writeFileAtomic(path, []byte("Delete this file to lock PSP games again.\n"))
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	// A PSP game on the TV leaves it as soon as PSP is locked.
	if current := s.currentGame(); current.ID != "" {
		if c, ok := s.cartByID(current.ID); ok && pspLocked(c, false) {
			s.selectGame("", true)
		}
	}
	return nil
}

type pspLockState struct {
	Unlocked bool   `json:"unlocked"`
	Locked   []cart `json:"locked"` // PSP games in the library while it is locked
}

func (s *APIServer) pspState() pspLockState {
	unlocked := s.pspUnlocked()
	state := pspLockState{Unlocked: unlocked, Locked: []cart{}}
	for _, c := range s.libraryCarts() {
		if pspLocked(c, unlocked) {
			state.Locked = append(state.Locked, c)
		}
	}
	return state
}

func (s *APIServer) getPSPLock(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.pspState())
}

func (s *APIServer) savePSPLock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Unlocked *bool `json:"unlocked"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil || body.Unlocked == nil {
		writeError(w, http.StatusBadRequest, "expected {\"unlocked\": true|false}")
		return
	}
	if err := s.setPSPUnlocked(*body.Unlocked); err != nil {
		writeError(w, http.StatusInternalServerError, "could not change the PSP lock")
		return
	}
	writeJSON(w, http.StatusOK, s.pspState())
}
