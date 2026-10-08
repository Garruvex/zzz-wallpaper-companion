package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func dateRequest(s *APIServer, method, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1/api/v1/dates", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	return w
}
func TestDatesPersistenceAndConflict(t *testing.T) {
	s := testServer(t)
	body := `{"version":1,"revision":0,"events":[{"id":"a","name":"Alex","date":"2000-02-29","kind":"birthday","yearly":true}]}`
	w := dateRequest(s, http.MethodPost, body)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var d DateDocument
	json.Unmarshal(w.Body.Bytes(), &d)
	if d.Revision != 1 {
		t.Fatal(d)
	}
	reloaded := newDateStore(s.dates.path)
	if reloaded.loadError != nil || len(reloaded.data.Events) != 1 {
		t.Fatal(reloaded)
	}
	w = dateRequest(s, http.MethodPost, body)
	if w.Code != 409 {
		t.Fatal(w.Code)
	}
	w = dateRequest(s, http.MethodGet, "")
	if !strings.Contains(w.Body.String(), "Alex") {
		t.Fatal(w.Body.String())
	}
}
func TestInvalidDatesPreserveSavedList(t *testing.T) {
	s := testServer(t)
	for _, body := range []string{`{"version":1,"revision":0,"events":[{"id":"a","name":"x","date":"2026-02-29","kind":"birthday"}]}`, `{"version":1,"revision":0,"events":null}`, `{"version":1,"revision":0,"events":[]} {}`} {
		w := dateRequest(s, http.MethodPost, body)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if s.dates.data.Revision != 0 {
		t.Fatal("changed invalid list")
	}
}
func TestCorruptDateFileIsPreserved(t *testing.T) {
	s := testServer(t)
	os.WriteFile(s.dates.path, []byte("broken"), 0600)
	s.dates = newDateStore(s.dates.path)
	w := dateRequest(s, http.MethodGet, "")
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
	w = dateRequest(s, http.MethodPost, `{"version":1,"revision":0,"events":[]}`)
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
	b, _ := os.ReadFile(s.dates.path)
	if string(b) != "broken" {
		t.Fatal("overwrote corrupt file")
	}
}
func TestDatesRejectRemoteClientsAndOrigins(t *testing.T) {
	s := testServer(t)
	for _, item := range []struct{ remote, origin string }{{"192.0.2.1:1234", ""}, {"127.0.0.1:1234", "https://example.com"}, {"127.0.0.1:1234", "http://127.0.0.1:8765.evil.example"}, {"127.0.0.1:1234", "http://localhost:8765@evil.example"}} {
		r := httptest.NewRequest("GET", "http://127.0.0.1/api/v1/dates", nil)
		r.RemoteAddr = item.remote
		r.Header.Set("Origin", item.origin)
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
}
