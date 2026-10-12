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

// serveROM sends a Game Boy ROM from a path on this PC. Wallpaper Engine's
// browser may not read local files itself, so the wallpaper asks here instead.
// Only existing ROM files are served, never other file types.
func (s *APIServer) serveROM(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path")), `"`)
	if path == "" || !filepath.IsAbs(path) || !romExtensions[strings.ToLower(filepath.Ext(path))] {
		writeError(w, http.StatusBadRequest, "path must be an absolute .gba, .gbc or .gb file")
		return
	}
	file, err := os.Open(path)
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
