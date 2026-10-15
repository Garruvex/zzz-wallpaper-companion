package companion

import (
	"crypto/sha1"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The Games page manages the cartridges: add a ROM, send a game to the TV,
// and reset, import, download or roll back its saves. The wallpaper polls
// the selection and follows it.

//go:embed games.html
var gamesHTML string

func (s *APIServer) gamesPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, gamesHTML)
}

// gameSelection is the game the TV should play. Seq changes on every
// request, so the wallpaper notices a repeat of the same game too. Fresh
// tells it to drop what it has in memory without saving, because the
// game's saves were just changed on disk.
type gameSelection struct {
	ID    string `json:"id"`
	Seq   uint64 `json:"seq"`
	Fresh bool   `json:"fresh"`
}

func (s *APIServer) selectGame(id string, fresh bool) gameSelection {
	s.gamesMu.Lock()
	defer s.gamesMu.Unlock()
	s.selection = gameSelection{ID: id, Seq: s.selection.Seq + 1, Fresh: fresh}
	return s.selection
}

func (s *APIServer) currentGame() gameSelection {
	s.gamesMu.Lock()
	defer s.gamesMu.Unlock()
	return s.selection
}

func (s *APIServer) getCurrentGame(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.currentGame())
}

func (s *APIServer) setCurrentGame(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "expected {\"id\": \"...\"}")
		return
	}
	if body.ID != "" {
		c, ok := s.cartByID(body.ID)
		if !ok {
			writeError(w, http.StatusNotFound, "no such game")
			return
		}
		if c.System == "psp" {
			writeError(w, http.StatusConflict, "PSP is no longer supported; existing games and saves are preserved")
			return
		}
	}
	writeJSON(w, http.StatusOK, s.selectGame(body.ID, false))
}

// cartByID finds a cartridge by folder name, refusing anything that isn't a
// plain folder name.
func (s *APIServer) cartByID(id string) (cart, bool) {
	if id == "" || id != filepath.Base(id) || strings.HasPrefix(id, ".") {
		return cart{}, false
	}
	return readCart(s.ensureROMDir(), id)
}

// cartMeta reads cart.json as a map so fields this version doesn't know
// about survive a rewrite.
func cartMeta(dir string) map[string]any {
	meta := map[string]any{}
	if data, err := os.ReadFile(filepath.Join(dir, "cart.json")); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	return meta
}

func writeCartMeta(dir string, meta map[string]any) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, "cart.json"), data)
}

// bumpRevision marks the cartridge's saves as changed outside the game. The
// wallpaper sends the revision it loaded with each save, so a game still
// running from before can't write its old progress over a reset or import.
func bumpRevision(c cart) (uint64, error) {
	meta := cartMeta(c.dir)
	rev := c.Rev + 1
	meta["rev"] = rev
	return rev, writeCartMeta(c.dir, meta)
}

// romHash returns the ROM's SHA-1, computing it once and keeping it in
// cart.json. It identifies the same game under a different file name.
func romHash(c cart) string {
	if c.SHA1 != "" {
		return c.SHA1
	}
	file, err := os.Open(filepath.Join(c.dir, c.ROM))
	if err != nil {
		return ""
	}
	defer file.Close()
	hash := sha1.New()
	if _, err := io.Copy(hash, io.LimitReader(file, maxROMBytes)); err != nil {
		return ""
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	meta := cartMeta(c.dir)
	meta["sha1"] = sum
	_ = writeCartMeta(c.dir, meta)
	return sum
}

var unsafeNameChars = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

// addGame stores an uploaded ROM as a new cartridge. If the same ROM is
// already in the library, that cartridge (and its saves) is returned instead.
func (s *APIServer) addGame(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Query().Get("name"))
	sys, ok := romSystem(name)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported game file; use a cartridge ROM, PS1 single image (.bin .img .pbp .chd)")
		return
	}
	// Stream large disc uploads to disk while hashing instead of allocating
	// an image-sized buffer in Companion.
	upload, err := os.CreateTemp(s.ensureROMDir(), ".upload-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not stage game upload")
		return
	}
	defer os.Remove(upload.Name())
	defer upload.Close()
	limit := romByteLimit(sys.ID)
	hasher := sha1.New()
	n, err := io.Copy(io.MultiWriter(upload, hasher), http.MaxBytesReader(w, r.Body, limit))
	if err != nil || n == 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("game file must be 1 byte to %d MiB", limit>>20))
		return
	}
	if err := upload.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not stage game upload")
		return
	}
	hash := hex.EncodeToString(hasher.Sum(nil))
	for _, c := range s.cartList() {
		if romHash(c) == hash {
			c.SHA1 = hash
			writeJSON(w, http.StatusOK, map[string]any{"cart": c, "existing": true})
			return
		}
	}
	root := s.ensureROMDir()
	base := strings.TrimSpace(unsafeNameChars.ReplaceAllString(strings.TrimSuffix(name, filepath.Ext(name)), ""))
	base = strings.TrimRight(base, ". ")
	if base == "" || strings.HasPrefix(base, ".") {
		base = "Game"
	}
	id := base
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(root, id)); os.IsNotExist(err) {
			break
		}
		id = fmt.Sprintf("%s (%d)", base, n)
	}
	dir := filepath.Join(root, id)
	if err := os.Mkdir(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create the game folder")
		return
	}
	if err := os.Rename(upload.Name(), filepath.Join(dir, "game"+strings.ToLower(filepath.Ext(name)))); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write the game")
		return
	}
	_ = writeCartMeta(dir, map[string]any{"sha1": hash})
	c, _ := readCart(root, id)
	writeJSON(w, http.StatusCreated, map[string]any{"cart": c, "existing": false})
}

type saveFile struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

func fileInfo(dir, name string) *saveFile {
	info, err := os.Stat(filepath.Join(dir, name))
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	return &saveFile{Name: name, Size: info.Size(), Modified: info.ModTime()}
}

// gameDetails lists a cartridge's save files for the Games page.
func (s *APIServer) gameDetails(w http.ResponseWriter, r *http.Request) {
	c, ok := s.cartByID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such game")
		return
	}
	states := []map[string]any{}
	for n := 1; n <= stateBackups; n++ {
		if f := fileInfo(c.dir, fmt.Sprintf("state-%d.state", n)); f != nil {
			states = append(states, map[string]any{"n": n, "modified": f.Modified})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cart":       c,
		"save":       fileInfo(c.dir, c.save),
		"saveBackup": fileInfo(c.dir, c.save+".bak"),
		"state":      fileInfo(c.dir, "state.state"),
		"states":     states,
	})
}

// shelve moves the named files of a cartridge into backups/<time>/, so a
// reset or replaced save can still be found by hand.
func shelve(c cart, names ...string) error {
	dest := filepath.Join(c.dir, "backups", time.Now().Format("2006-01-02 150405"))
	moved := false
	for _, name := range names {
		from := filepath.Join(c.dir, name)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if !moved {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			moved = true
		}
		if err := os.Rename(from, filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	return nil
}

func stateNames() []string {
	names := []string{"state.state"}
	for n := 1; n <= stateBackups; n++ {
		names = append(names, fmt.Sprintf("state-%d.state", n))
	}
	return names
}

// changedSaves finishes an action that changed a game's saves: the
// revision moves on, and a game on the TV reloads without saving over it.
func (s *APIServer) changedSaves(w http.ResponseWriter, c cart) {
	rev, err := bumpRevision(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update the game")
		return
	}
	if s.currentGame().ID == c.ID {
		s.selectGame(c.ID, true)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rev": rev})
}

// gameAction runs one of the Games page's save actions:
//
//	reset    shelve the save and every resume point; the game starts new
//	restore  ?state=N makes resume point N current (the current one is shelved)
//	import   the body becomes the game's save; resume points are shelved so
//	         the game boots from it
//	remove   move the whole cartridge to roms/.removed
func (s *APIServer) gameAction(w http.ResponseWriter, r *http.Request) {
	c, ok := s.cartByID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such game")
		return
	}
	switch r.PathValue("action") {
	case "reset":
		if err := shelve(c, append([]string{c.save, c.save + ".bak"}, stateNames()...)...); err != nil {
			writeError(w, http.StatusInternalServerError, "could not move the saves aside")
			return
		}
		s.changedSaves(w, c)
	case "restore":
		n, err := strconv.Atoi(r.URL.Query().Get("state"))
		from := fmt.Sprintf("state-%d.state", n)
		if err != nil || n < 1 || n > stateBackups || fileInfo(c.dir, from) == nil {
			writeError(w, http.StatusNotFound, "no such resume point")
			return
		}
		data, err := os.ReadFile(filepath.Join(c.dir, from))
		if err == nil {
			err = shelve(c, "state.state")
		}
		if err == nil {
			err = writeFileAtomic(filepath.Join(c.dir, "state.state"), data)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not restore it")
			return
		}
		s.changedSaves(w, c)
	case "import":
		limit := cartByteLimit(c, "save")
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
		if err != nil || len(data) == 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("save file must be 1 byte to %d MiB", limit>>20))
			return
		}
		if err := shelve(c, append([]string{c.save}, stateNames()...)...); err != nil {
			writeError(w, http.StatusInternalServerError, "could not move the old save aside")
			return
		}
		if err := writeFileAtomic(filepath.Join(c.dir, c.save), data); err != nil {
			writeError(w, http.StatusInternalServerError, "could not write the save")
			return
		}
		s.changedSaves(w, c)
	case "shelf":
		s.setShelf(w, r, c)
	case "remove":
		removed := filepath.Join(s.romDir(), ".removed")
		if err := os.MkdirAll(removed, 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, "could not remove it")
			return
		}
		dest := filepath.Join(removed, c.ID+" "+time.Now().Format("2006-01-02 150405"))
		if err := os.Rename(c.dir, dest); err != nil {
			writeError(w, http.StatusInternalServerError, "could not remove it (is a file open?)")
			return
		}
		if s.currentGame().ID == c.ID {
			s.selectGame("", true)
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusNotFound, "unknown action")
	}
}

// shelfSlots is how many tapes the wallpaper's shelf has.
const shelfSlots = 14

// applyShelf decides which games get a tape. Games the player put on the
// shelf come first, then games that were never asked about fill the free
// tapes, in title order; games the player took off never get one. carts must
// already be in title order.
func applyShelf(carts []cart) {
	left := shelfSlots
	for _, wanted := range []bool{true, false} {
		for i := range carts {
			c := &carts[i]
			if c.System == "psp" {
				c.OnShelf = false
				continue
			}
			explicit := c.Shelf != nil && *c.Shelf
			unset := c.Shelf == nil
			if left > 0 && ((wanted && explicit) || (!wanted && unset)) {
				c.OnShelf = true
				left--
			}
		}
	}
}

// setShelf puts a game on the shelf or takes it off.
func (s *APIServer) setShelf(w http.ResponseWriter, r *http.Request, c cart) {
	on := r.URL.Query().Get("on") == "1"
	if on && c.System == "psp" {
		writeError(w, http.StatusBadRequest, "PSP is no longer supported")
		return
	}
	if on {
		taken := 0
		for _, other := range s.cartList() {
			if other.System != "psp" && other.ID != c.ID && other.Shelf != nil && *other.Shelf {
				taken++
			}
		}
		if taken >= shelfSlots {
			writeError(w, http.StatusConflict, fmt.Sprintf("The shelf is full (%d tapes). Take another game off it first.", shelfSlots))
			return
		}
	}
	meta := cartMeta(c.dir)
	meta["shelf"] = on
	if err := writeCartMeta(c.dir, meta); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update the game")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

var errStaleRevision = errors.New("the game's saves changed since it was loaded")

// checkRevision rejects a save from a game loaded before its saves were
// reset or replaced. Wallpapers that don't send a revision are allowed.
func checkRevision(r *http.Request, c cart) error {
	sent := r.Header.Get("X-Cart-Rev")
	if sent == "" {
		return nil
	}
	if rev, err := strconv.ParseUint(sent, 10, 64); err != nil || rev != c.Rev {
		return errStaleRevision
	}
	return nil
}
