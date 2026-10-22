package companion

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Garruvex/zzz-wallpaper-companion/internal/gamepad"
)

// PSP is not stable yet, so it stays locked. While it is locked the companion
// takes no PSP files and never hands a PSP game to the wallpaper: the
// cartridge list leaves them out, and they can't be played, put on the shelf
// or have their ROM read. PSP games already in the library keep their folders
// and saves, and the Games page shows them greyed out.
//
// There is one way to change it, and nothing in the API or the pages offers
// it: a button sequence on a controller while the game station is on with no
// tape in the deck. Only its hash is kept here. The same sequence locks it
// again. The unlock is a marker file next to the settings, so it survives
// restarts and updates.

const pspUnlockFile = "psp-unlocked"

func (s *APIServer) pspUnlocked() bool {
	_, err := os.Stat(filepath.Join(s.dataDir, pspUnlockFile))
	return err == nil
}

func pspLocked(c cart, unlocked bool) bool { return c.System == "psp" && !unlocked }

func (s *APIServer) setPSPUnlocked(on bool) error {
	path := filepath.Join(s.dataDir, pspUnlockFile)
	if on {
		if err := writeFileAtomic(path, []byte("Delete this file to lock PSP games again.\n")); err != nil {
			return err
		}
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	s.input.SetPSPUnlocked(on)
	// A PSP game on the TV leaves it as soon as PSP is locked.
	if current := s.currentGame(); !on && current.ID != "" {
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

const (
	pspCodeHash  = "22d61fc1428698ff8c0f06c42bee8b53c33476f13127e29c288ad7acd3313d52"
	pspCodeLen   = 10
	pspCodeGap   = 1500 * time.Millisecond
	pspCodeOther = "other"
)

var pspCodeButtons = []struct {
	bit  int
	name string
}{
	{gamepad.ButtonUp, "up"}, {gamepad.ButtonDown, "down"}, {gamepad.ButtonLeft, "left"},
	{gamepad.ButtonRight, "right"}, {gamepad.ButtonB, "b"}, {gamepad.ButtonA, "a"},
}

// pspCode follows controller presses the game isn't using. Each press has to
// come within pspCodeGap of the one before, or the sequence starts over.
type pspCode struct {
	mu      sync.Mutex
	held    uint32
	presses []string
	last    time.Time
	now     func() time.Time
}

// watchPSPCode gets the controller's buttons while no game has them. It only
// counts with no tape in the deck.
func (s *APIServer) watchPSPCode(buttons uint32) {
	code := &s.pspCode
	code.mu.Lock()
	pressed := buttons &^ code.held
	code.held = buttons
	if pressed == 0 {
		code.mu.Unlock()
		return
	}
	if s.currentGame().ID != "" {
		code.presses = nil
		code.mu.Unlock()
		return
	}
	now := time.Now()
	if code.now != nil {
		now = code.now()
	}
	if now.Sub(code.last) > pspCodeGap {
		code.presses = nil
	}
	code.last = now
	for bit := 0; bit < 32; bit++ {
		if pressed&(1<<bit) == 0 {
			continue
		}
		name := pspCodeOther
		for _, button := range pspCodeButtons {
			if button.bit == bit {
				name = button.name
			}
		}
		code.presses = append(code.presses, name)
	}
	if len(code.presses) > pspCodeLen {
		code.presses = code.presses[len(code.presses)-pspCodeLen:]
	}
	sum := sha256.Sum256([]byte(strings.Join(code.presses, " ")))
	matched := len(code.presses) == pspCodeLen && hex.EncodeToString(sum[:]) == pspCodeHash
	if matched {
		code.presses = nil
	}
	code.mu.Unlock()
	if matched {
		_ = s.setPSPUnlocked(!s.pspUnlocked())
	}
}
