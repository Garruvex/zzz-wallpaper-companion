package companion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A cartridge is one folder in the ROM folder that holds everything for one
// game, so copying the folder takes the game and its progress along:
//
//	Pokemon Emerald/
//	  game.gba      the ROM; its extension picks the system
//	  save.sav      the game's own save (save.srm for SNES and Sega)
//	  state.state   where the player left off, with state-1..3 as older copies
//	  label.png     optional label art
//	  cart.json     optional {"title": "...", "color": "#rrggbb"}
//
// ROM files dropped loose into the ROM folder are moved into a cartridge of
// their own the next time the wallpaper lists cartridges.

type system struct {
	ID      string // the emulator's system name
	SaveExt string // the name desktop emulators use for battery saves
}

var systemsByExt = map[string]system{
	".gba": {"gba", ".sav"}, ".gbc": {"gba", ".sav"}, ".gb": {"gba", ".sav"},
	".nes": {"nes", ".sav"},
	".sfc": {"snes", ".srm"}, ".smc": {"snes", ".srm"},
	".md": {"segaMD", ".srm"}, ".gen": {"segaMD", ".srm"}, ".smd": {"segaMD", ".srm"},
	".sms": {"segaMS", ".srm"}, ".gg": {"segaGG", ".srm"},
}

const (
	maxROMBytes      = 64 << 20
	maxCartFileBytes = 8 << 20
	stateBackups     = 3
	// A new backup is taken when the current state is at least this old, so
	// the backups reach back in time instead of all being 30 seconds apart.
	stateBackupAge = 10 * time.Minute
)

type cart struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	System string `json:"system"`
	ROM    string `json:"rom"`
	Color  string `json:"color,omitempty"`
	Label  bool   `json:"label"`
	Rev    uint64 `json:"rev"`            // changes when the saves are reset or replaced
	SHA1   string `json:"sha1,omitempty"` // the ROM's hash, once known
	dir    string
	save   string
}

func (s *APIServer) romDir() string { return filepath.Join(s.dataDir, "roms") }

func (s *APIServer) ensureROMDir() string {
	dir := s.romDir()
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func romSystem(name string) (system, bool) {
	sys, ok := systemsByExt[strings.ToLower(filepath.Ext(name))]
	return sys, ok
}

// readCart describes the cartridge in dir, or returns false if it holds no ROM.
func readCart(root, id string) (cart, bool) {
	dir := filepath.Join(root, id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return cart{}, false
	}
	c := cart{ID: id, Title: id, dir: dir}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if sys, ok := romSystem(name); ok && c.ROM == "" {
			c.ROM, c.System, c.save = name, sys.ID, "save"+sys.SaveExt
		}
		if strings.EqualFold(name, "label.png") {
			c.Label = true
		}
	}
	if c.ROM == "" {
		return cart{}, false
	}
	if data, err := os.ReadFile(filepath.Join(dir, "cart.json")); err == nil {
		var meta struct {
			Title, Color, SHA1 string
			Rev                uint64
		}
		if json.Unmarshal(data, &meta) == nil {
			if t := strings.TrimSpace(meta.Title); t != "" {
				c.Title = t
			}
			if len(meta.Color) == 7 && meta.Color[0] == '#' {
				c.Color = meta.Color
			}
			c.Rev = meta.Rev
			if len(meta.SHA1) == 40 {
				c.SHA1 = meta.SHA1
			}
		}
	}
	return c, true
}

// wrapLooseROMs gives every ROM file sitting directly in the ROM folder its
// own cartridge folder, bringing along saves made by earlier versions
// (Game.sav, Game.sav.bak, Game.state).
func wrapLooseROMs(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		sys, ok := romSystem(entry.Name())
		if !ok || !entry.Type().IsRegular() {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		id := base
		for n := 2; ; n++ {
			if _, err := os.Stat(filepath.Join(root, id)); os.IsNotExist(err) {
				break
			}
			id = fmt.Sprintf("%s (%d)", base, n)
		}
		dir := filepath.Join(root, id)
		if err := os.Mkdir(dir, 0o755); err != nil {
			log.Printf("cartridge %q: %v", id, err)
			continue
		}
		moves := [][2]string{
			{entry.Name(), "game" + strings.ToLower(filepath.Ext(entry.Name()))},
			{base + ".sav", "save" + sys.SaveExt},
			{base + ".sav.bak", "save" + sys.SaveExt + ".bak"},
			{base + ".state", "state.state"},
		}
		for _, move := range moves {
			from := filepath.Join(root, move[0])
			if _, err := os.Stat(from); err != nil {
				continue
			}
			if err := os.Rename(from, filepath.Join(dir, move[1])); err != nil {
				log.Printf("cartridge %q: moving %s: %v", id, move[0], err)
			}
		}
	}
}

func (s *APIServer) cartList() []cart {
	root := s.ensureROMDir()
	wrapLooseROMs(root)
	entries, _ := os.ReadDir(root)
	carts := []cart{}
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			if c, ok := readCart(root, entry.Name()); ok {
				carts = append(carts, c)
			}
		}
	}
	sort.Slice(carts, func(i, j int) bool { return strings.ToLower(carts[i].Title) < strings.ToLower(carts[j].Title) })
	return carts
}

func (s *APIServer) listCarts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"carts": s.cartList()})
}

// cartFile maps a request to a file inside one cartridge. Only these names
// can be reached: rom, save, state and label.
func (s *APIServer) cartFile(r *http.Request) (c cart, path, kind string, ok bool) {
	id := r.PathValue("id")
	if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") {
		return cart{}, "", "", false
	}
	c, ok = readCart(s.ensureROMDir(), id)
	if !ok {
		return cart{}, "", "", false
	}
	switch kind = r.PathValue("file"); kind {
	case "rom":
		path = c.ROM
	case "save":
		path = c.save
	case "state":
		path = "state.state"
	case "label":
		path = "label.png"
	default:
		return cart{}, "", "", false
	}
	return c, filepath.Join(c.dir, path), kind, true
}

func (s *APIServer) getCartFile(w http.ResponseWriter, r *http.Request) {
	c, path, kind, ok := s.cartFile(r)
	if !ok {
		writeError(w, http.StatusNotFound, "no such cartridge or file")
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "not there yet")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxROMBytes {
		writeError(w, http.StatusNotFound, "not there yet")
		return
	}
	contentType := "application/octet-stream"
	if kind == "label" {
		contentType = "image/png"
	}
	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", c.Title+filepath.Ext(path)))
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, r, "", info.ModTime(), file)
}

func (s *APIServer) putCartFile(w http.ResponseWriter, r *http.Request) {
	c, path, kind, ok := s.cartFile(r)
	if !ok || (kind != "save" && kind != "state") {
		writeError(w, http.StatusNotFound, "only save and state can be written")
		return
	}
	if err := checkRevision(r, c); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCartFileBytes))
	if err != nil || len(data) == 0 {
		writeError(w, http.StatusBadRequest, "file must be 1 byte to 8 MB")
		return
	}
	if old, err := os.ReadFile(path); err == nil {
		if bytes.Equal(old, data) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if kind == "save" {
			// Keep the previous in-game save in case a bad write replaces a good one.
			_ = os.WriteFile(path+".bak", old, 0o644)
		} else if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) >= stateBackupAge {
			rotateStates(filepath.Dir(path))
		}
	}
	if err := writeFileAtomic(path, data); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write file")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rotateStates shifts state.state to state-1.state, state-1 to state-2, and so on.
func rotateStates(dir string) {
	name := func(n int) string {
		if n == 0 {
			return filepath.Join(dir, "state.state")
		}
		return filepath.Join(dir, fmt.Sprintf("state-%d.state", n))
	}
	for n := stateBackups - 1; n >= 0; n-- {
		if _, err := os.Stat(name(n)); err == nil {
			_ = os.Rename(name(n), name(n+1))
		}
	}
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
