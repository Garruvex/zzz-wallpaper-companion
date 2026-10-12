package companion

import (
	"bufio"
	"net/http"
	"net/http/httptest"
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
	for _, body := range []string{`{"enabled":true,"x":1}`, `{"enabled":true,"capture":"sometimes"}`} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/input/play-mode", strings.NewReader(body))
		request.RemoteAddr = "127.0.0.1:1234"
		recorder := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", body, recorder.Code)
		}
	}
}
