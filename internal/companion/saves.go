package companion

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Game saves live next to the ROM so they survive Wallpaper Engine cache
// clears and can be backed up: Game.sav is the game's own save (the same file
// desktop mGBA uses) and Game.state is the wallpaper's resume snapshot.
var saveKinds = map[string]string{"save": ".sav", "state": ".state"}

const maxSaveBytes = 8 << 20

// savePath returns where a ROM's save of the given kind lives, or "" if the
// request doesn't name an existing ROM in the ROM folder.
func (s *APIServer) savePath(r *http.Request) string {
	name, ext := r.PathValue("name"), saveKinds[r.PathValue("kind")]
	if ext == "" || name == "" || name != filepath.Base(name) || !romExtensions[strings.ToLower(filepath.Ext(name))] {
		return ""
	}
	rom := filepath.Join(s.ensureROMDir(), name)
	if info, err := os.Stat(rom); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return strings.TrimSuffix(rom, filepath.Ext(rom)) + ext
}

func (s *APIServer) getSave(w http.ResponseWriter, r *http.Request) {
	path := s.savePath(r)
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid ROM or save kind")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "no save yet")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}

func (s *APIServer) putSave(w http.ResponseWriter, r *http.Request) {
	path := s.savePath(r)
	if path == "" {
		writeError(w, http.StatusBadRequest, "invalid ROM or save kind")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSaveBytes))
	if err != nil || len(data) == 0 {
		writeError(w, http.StatusBadRequest, "save must be 1 byte to 8 MB")
		return
	}
	if old, err := os.ReadFile(path); err == nil {
		if bytes.Equal(old, data) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.PathValue("kind") == "save" {
			// Keep the previous in-game save in case a bad write replaces a good one.
			_ = os.WriteFile(path+".bak", old, 0o644)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, "could not write save")
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, "could not write save")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
