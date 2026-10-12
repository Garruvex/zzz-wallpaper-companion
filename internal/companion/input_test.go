package companion

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInputStreamSendsInitialState(t *testing.T) {
	server := testServer(t)
	ts := httptest.NewServer(server.http.Handler)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/input")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				line <- scanner.Text()
				return
			}
		}
	}()
	select {
	case got := <-line:
		if !strings.Contains(got, `"playMode":false`) {
			t.Fatalf("unexpected event %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event received")
	}
}

func TestPlayModeRejectsBadBody(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/input/play-mode", strings.NewReader(`{"enabled":true,"x":1}`))
	request.RemoteAddr = "127.0.0.1:1234"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status %d", recorder.Code)
	}
}

func TestServeROMOnlyServesROMFiles(t *testing.T) {
	server := testServer(t)
	dir := t.TempDir()
	rom := filepath.Join(dir, "Test Game.gba")
	if err := os.WriteFile(rom, []byte("ROMDATA"), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/rom?path="+url.QueryEscape(path), nil)
		request.RemoteAddr = "127.0.0.1:1234"
		recorder := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(recorder, request)
		return recorder
	}
	if r := get(rom); r.Code != http.StatusOK || r.Body.String() != "ROMDATA" {
		t.Fatalf("rom: %d %q", r.Code, r.Body.String())
	}
	if r := get(`"` + rom + `"`); r.Code != http.StatusOK {
		t.Fatalf("quoted path: %d", r.Code)
	}
	for _, bad := range []string{secret, "relative.gba", filepath.Join(dir, "missing.gba"), ""} {
		if r := get(bad); r.Code == http.StatusOK {
			t.Fatalf("served %q", bad)
		}
	}
}
