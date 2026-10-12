package companion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const inputKeepAlive = 15 * time.Second

// inputStream sends controller state to the wallpaper as server-sent events.
// The hub only polls hardware while at least one stream is open.
func (s *APIServer) inputStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	updates, cancel := s.input.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	keepAlive := time.NewTicker(inputKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case snapshot := <-updates:
			body, _ := json.Marshal(snapshot)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// setPlayMode turns keyboard capture on or off. It only takes effect while a
// wallpaper is listening to the stream, and it resets when the last one leaves.
func (s *APIServer) setPlayMode(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var request struct {
		Enabled bool `json:"enabled"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid play mode request")
		return
	}
	writeJSON(w, http.StatusOK, s.input.SetPlayMode(request.Enabled))
}

const maxROMBytes = 64 << 20

var romExtensions = map[string]bool{".gba": true, ".gbc": true, ".gb": true}

// romDir is the folder the user drops their own games into.
func (s *APIServer) romDir() string { return filepath.Join(s.dataDir, "roms") }

func (s *APIServer) ensureROMDir() string {
	dir := s.romDir()
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// listROMs names the ROM files in the ROM folder. The wallpaper shows one
// channel per name.
func (s *APIServer) listROMs(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(s.ensureROMDir())
	roms := []string{}
	for _, entry := range entries {
		if entry.Type().IsRegular() && romExtensions[strings.ToLower(filepath.Ext(entry.Name()))] {
			roms = append(roms, entry.Name())
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"roms": roms})
}

// serveROM sends one ROM from the ROM folder by file name. Nothing outside
// that folder, and no other file type, can be requested.
func (s *APIServer) serveROM(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" || name != filepath.Base(name) || !romExtensions[strings.ToLower(filepath.Ext(name))] {
		writeError(w, http.StatusBadRequest, "invalid ROM name")
		return
	}
	file, err := os.Open(filepath.Join(s.ensureROMDir(), name))
	if err != nil {
		writeError(w, http.StatusNotFound, "ROM not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "ROM not found")
		return
	}
	if info.Size() > maxROMBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "ROM is too large")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "", info.ModTime(), file)
}
