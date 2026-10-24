package companion

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Garruvex/zzz-wallpaper-companion/internal/gamepad"
)

// The keyboard controls for the game station can be changed on the Game tab's Keyboard page. They are
// kept in keys.json next to the settings as button name to Windows virtual-key code (0 for no key),
// and are a page helper for this window, not part of the public API. Controllers need no setting:
// their buttons are taken by position.

const keysFile = "keys.json"

var keyButtons = []struct {
	name   string
	button uint
}{
	{"up", gamepad.ButtonUp}, {"down", gamepad.ButtonDown}, {"left", gamepad.ButtonLeft}, {"right", gamepad.ButtonRight},
	{"a", gamepad.ButtonA}, {"b", gamepad.ButtonB}, {"x", gamepad.ButtonX}, {"y", gamepad.ButtonY},
	{"l", gamepad.ButtonL}, {"r", gamepad.ButtonR}, {"start", gamepad.ButtonStart}, {"select", gamepad.ButtonSelect},
}

// Keys that can't be a game key: Pause starts and stops play, Escape cancels choosing a key, and
// Shift, Ctrl, Alt and the Windows keys make shortcuts, which always go to the system.
var reservedKeys = map[uint32]bool{
	0x13: true, 0x1B: true, 0x10: true, 0x11: true, 0x12: true, 0x5B: true, 0x5C: true,
	0xA0: true, 0xA1: true, 0xA2: true, 0xA3: true, 0xA4: true, 0xA5: true,
}

// keysByName turns virtual-key-to-button bindings into the page's button-to-key form.
func keysByName(bindings map[uint32]uint) map[string]uint32 {
	out := make(map[string]uint32, len(keyButtons))
	for _, key := range keyButtons {
		out[key.name] = 0
		for vk, button := range bindings {
			if button == key.button {
				out[key.name] = vk
			}
		}
	}
	return out
}

// keyBindings checks the page's form: known buttons only, keys from 1 to 254 that aren't reserved,
// and no key on two buttons.
func keyBindings(byName map[string]uint32) (map[uint32]uint, bool) {
	bindings := map[uint32]uint{}
	for name, vk := range byName {
		var button uint
		known := false
		for _, key := range keyButtons {
			if key.name == name {
				button, known = key.button, true
			}
		}
		if !known || vk > 254 || reservedKeys[vk] {
			return nil, false
		}
		if vk == 0 {
			continue
		}
		if _, taken := bindings[vk]; taken {
			return nil, false
		}
		bindings[vk] = button
	}
	return bindings, true
}

// loadKeys applies the saved keys at start; with none saved (or a bad file) the defaults stay.
func (s *APIServer) loadKeys() {
	data, err := os.ReadFile(filepath.Join(s.dataDir, keysFile))
	if err != nil {
		return
	}
	var byName map[string]uint32
	if json.Unmarshal(data, &byName) != nil {
		return
	}
	if bindings, ok := keyBindings(byName); ok {
		gamepad.SetKeyBindings(bindings)
	}
}

func (s *APIServer) keysState(w http.ResponseWriter) {
	writeJSON(w, http.StatusOK, map[string]any{
		"keys":     keysByName(gamepad.KeyBindings()),
		"defaults": keysByName(gamepad.DefaultKeyBindings()),
	})
}

func (s *APIServer) getKeys(w http.ResponseWriter, _ *http.Request) { s.keysState(w) }

// saveKeys takes { "keys": { button: key } } for every button, or { "keys": null } for the defaults.
func (s *APIServer) saveKeys(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keys map[string]uint32 `json:"keys"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid keys")
		return
	}
	path := filepath.Join(s.dataDir, keysFile)
	if body.Keys == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			writeError(w, http.StatusInternalServerError, "Could not reset the keys.")
			return
		}
		gamepad.SetKeyBindings(nil)
		s.keysState(w)
		return
	}
	bindings, ok := keyBindings(body.Keys)
	if !ok || len(body.Keys) != len(keyButtons) {
		writeError(w, http.StatusBadRequest, "Each button needs its own key, and Pause, Escape, Shift, Ctrl, Alt and the Windows keys can't be used.")
		return
	}
	data, _ := json.MarshalIndent(keysByName(bindings), "", "  ")
	if err := writeFileAtomic(path, data); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save the keys.")
		return
	}
	gamepad.SetKeyBindings(bindings)
	s.keysState(w)
}
