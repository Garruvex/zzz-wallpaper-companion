package companion

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode"
)

// A game's name is what the Library, the wallpaper's tape tag and the tape labels show. It starts as the
// game file's name and is kept apart from the game's folder, so renaming changes only `title` in
// cart.json: the saves, the shelf tape and the id the wallpaper remembers stay where they are.

const maxTitleRunes = 40

// cleanTitle trims the name and drops control characters. An empty result means "use the file's name".
func cleanTitle(raw string) string {
	var out strings.Builder
	space := false
	for _, r := range strings.TrimSpace(raw) {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			space = out.Len() > 0
			continue
		}
		if space {
			out.WriteByte(' ')
			space = false
		}
		out.WriteRune(r)
	}
	if runes := []rune(out.String()); len(runes) > maxTitleRunes {
		return strings.TrimSpace(string(runes[:maxTitleRunes]))
	}
	return out.String()
}

func (s *APIServer) renameGame(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Could not rename the game.")
		return
	}
	c, ok := s.cartByID(body.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "That game is no longer in the Library.")
		return
	}
	meta := cartMeta(c.dir)
	if title := cleanTitle(body.Title); title != "" {
		meta["title"] = title
	} else {
		delete(meta, "title")
	}
	if err := writeCartMeta(c.dir, meta); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save the name.")
		return
	}
	c, _ = s.cartByID(body.ID)
	writeJSON(w, http.StatusOK, map[string]any{"cart": c})
}
