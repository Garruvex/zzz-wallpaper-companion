package companion

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Garruvex/zzz-wallpaper-companion/internal/gamepad"
)

func TestKeyboardKeysAreSavedAndApplied(t *testing.T) {
	t.Cleanup(func() { gamepad.SetKeyBindings(nil) })
	server := testServer(t)
	type state struct{ Keys, Defaults map[string]uint32 }
	read := func(body []byte) state {
		t.Helper()
		var got state
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	got := read(cartRequest(t, server, http.MethodGet, "/controller/keys", nil).Body.Bytes())
	if got.Keys["a"] != 0x58 || got.Defaults["start"] != 0x0D || len(got.Keys) != len(keyButtons) {
		t.Fatalf("defaults %+v", got)
	}

	// A is moved to J and Start loses its key.
	keys := got.Keys
	keys["a"], keys["start"] = 0x4A, 0
	body, _ := json.Marshal(map[string]any{"keys": keys})
	r := cartRequest(t, server, http.MethodPost, "/controller/keys", body)
	if r.Code != http.StatusOK || read(r.Body.Bytes()).Keys["a"] != 0x4A {
		t.Fatalf("save: %d %s", r.Code, r.Body)
	}
	if live := gamepad.KeyBindings(); live[0x4A] != gamepad.ButtonA || live[0x58] != 0 || len(live) != len(keyButtons)-1 {
		t.Fatalf("the hook's keys: %v", live)
	}

	// A key on two buttons, a reserved key, and an unknown button are refused.
	for _, bad := range []map[string]uint32{{"a": 0x4A, "b": 0x4A}, {"a": 0x13}, {"jump": 0x20}} {
		next := map[string]uint32{}
		for name, vk := range keys {
			next[name] = vk
		}
		for name, vk := range bad {
			next[name] = vk
		}
		body, _ := json.Marshal(map[string]any{"keys": next})
		if r := cartRequest(t, server, http.MethodPost, "/controller/keys", body); r.Code != http.StatusBadRequest {
			t.Errorf("accepted %v: %d", bad, r.Code)
		}
	}

	// The keys survive a restart, and null puts the defaults back.
	gamepad.SetKeyBindings(nil)
	server.loadKeys()
	if gamepad.KeyBindings()[0x4A] != gamepad.ButtonA {
		t.Fatal("saved keys not loaded")
	}
	if r := cartRequest(t, server, http.MethodPost, "/controller/keys", []byte(`{"keys":null}`)); r.Code != http.StatusOK || read(r.Body.Bytes()).Keys["a"] != 0x58 {
		t.Fatalf("reset: %d %s", r.Code, r.Body)
	}
	if _, err := os.Stat(filepath.Join(server.dataDir, keysFile)); !os.IsNotExist(err) {
		t.Fatal("reset left keys.json")
	}
}
