package companion

import (
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type ImportantDate struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Date   string `json:"date"`
	Kind   string `json:"kind"`
	Yearly bool   `json:"yearly"`
}
type DateDocument struct {
	Version  int             `json:"version"`
	Revision int             `json:"revision"`
	Events   []ImportantDate `json:"events"`
}
type DateStore struct {
	mu        sync.Mutex
	path      string
	data      DateDocument
	loadError error
}

func newDateStore(path string) *DateStore {
	s := &DateStore{path: path, data: DateDocument{Version: 1, Events: []ImportantDate{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err == nil {
		err = json.Unmarshal(b, &s.data)
	}
	if err == nil {
		err = validateDates(s.data)
	}
	s.loadError = err
	return s
}
func validateDates(d DateDocument) error {
	if d.Version != 1 || d.Revision < 0 || len(d.Events) > 500 || d.Events == nil {
		return errors.New("invalid date document (maximum 500 dates)")
	}
	ids := map[string]bool{}
	for _, e := range d.Events {
		parsed, err := time.Parse("2006-01-02", e.Date)
		if err != nil || parsed.Year() < 1000 || parsed.Year() > 9999 || e.ID == "" || len(e.ID) > 100 || ids[e.ID] || strings.TrimSpace(e.Name) == "" || utf8.RuneCountInString(e.Name) > 80 || (e.Kind != "birthday" && e.Kind != "custom") {
			return errors.New("each date needs a unique ID, a name, valid date and birthday/event type")
		}
		ids[e.ID] = true
	}
	return nil
}
func (s *APIServer) getDates(w http.ResponseWriter, r *http.Request) {
	s.dates.mu.Lock()
	defer s.dates.mu.Unlock()
	if s.dates.loadError != nil {
		writeError(w, 500, "Saved dates could not be read. Your file has been preserved.")
		return
	}
	writeJSON(w, 200, s.dates.data)
}
func (s *APIServer) saveDates(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	decoder.DisallowUnknownFields()
	var next DateDocument
	if err := decoder.Decode(&next); err != nil {
		writeError(w, 400, "invalid dates")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, 400, "unexpected trailing data")
		return
	}
	if err := validateDates(next); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	s.dates.mu.Lock()
	defer s.dates.mu.Unlock()
	if s.dates.loadError != nil {
		writeError(w, 500, "Saved dates could not be read. Your file has been preserved.")
		return
	}
	if next.Revision != s.dates.data.Revision {
		writeError(w, 409, "Dates changed in another window. Reload before saving.")
		return
	}
	next.Revision++
	for i := range next.Events {
		next.Events[i].Name = strings.TrimSpace(next.Events[i].Name)
	}
	b, err := json.MarshalIndent(next, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(s.dates.path), 0755)
	}
	var f *os.File
	if err == nil {
		f, err = os.CreateTemp(filepath.Dir(s.dates.path), "dates-*.tmp")
	}
	if err == nil {
		tmp := f.Name()
		defer os.Remove(tmp)
		err = f.Chmod(0600)
		if err == nil {
			_, err = f.Write(b)
		}
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmp, s.dates.path)
		}
	}
	if err != nil {
		writeError(w, 500, "Could not save dates. Previous dates are unchanged.")
		return
	}
	s.dates.data = next
	writeJSON(w, 200, next)
}

//go:embed dates.html
var datesHTML string

func (s *APIServer) datesPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, datesHTML)
}
