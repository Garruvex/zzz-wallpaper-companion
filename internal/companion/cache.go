package companion

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// The Games page's Storage card: what the companion keeps only because it never deletes on its own.
// Removed games (roms/.removed, with their saves) and the weather map's tile cache can be emptied
// there. Nothing a game still uses is touched: every game's folder, its saves and its backups stay.

type cacheUse struct {
	RemovedGames int   `json:"removedGames"`
	RemovedBytes int64 `json:"removedBytes"`
	MapBytes     int64 `json:"mapBytes"`
}

func dirBytes(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (s *APIServer) cacheUse() cacheUse {
	removed := filepath.Join(s.romDir(), ".removed")
	use := cacheUse{RemovedBytes: dirBytes(removed), MapBytes: dirBytes(filepath.Join(s.dataDir, "map-tiles"))}
	if entries, err := os.ReadDir(removed); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				use.RemovedGames++
			}
		}
	}
	return use
}

func (s *APIServer) getCache(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.cacheUse())
}

// clearCache deletes for good, so it only answers a request that says so.
func (s *APIServer) clearCache(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || !body.Confirm {
		writeError(w, http.StatusBadRequest, "clearing the cache must be confirmed")
		return
	}
	before := s.cacheUse()
	for _, dir := range []string{filepath.Join(s.romDir(), ".removed"), filepath.Join(s.dataDir, "map-tiles")} {
		if err := os.RemoveAll(dir); err != nil {
			writeError(w, http.StatusInternalServerError, "could not clear everything (is a file open?)")
			return
		}
	}
	writeJSON(w, http.StatusOK, before)
}
