package companion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Garruvex/zzz-wallpaper-companion/internal/systemstats"
)

func testServer(t *testing.T) *APIServer {
	t.Helper()
	store, err := newConfigStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	server := newAPIServer(store, newResolver(dataDir, store), newFFmpegManager(dataDir))
	t.Cleanup(server.systemStats.Close)
	return server
}

func TestHealth(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/health", nil)
	request.RemoteAddr = "127.0.0.1:50000"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["name"] != "zzz-wallpaper-companion" {
		t.Fatalf("unexpected body: %v", body)
	}
	capabilities, ok := body["capabilities"].(map[string]any)
	if !ok || capabilities["lyrics"] != true || capabilities["systemStats"] != true {
		t.Fatalf("missing capabilities: %v", body)
	}
}

func TestSystemStatsEndpoint(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/system-stats", nil)
	request.RemoteAddr = "127.0.0.1:50000"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var body systemstats.SystemStatsSnapshot
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SampledAt <= 0 {
		t.Fatalf("invalid snapshot: %#v", body)
	}
}

func TestRejectsRemoteClients(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/health", nil)
	request.RemoteAddr = "192.0.2.10:50000"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("got %d", recorder.Code)
	}
}

func TestSaveSettings(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/settings", strings.NewReader(`{"port":9000,"maxHeight":1080,"updateChannel":"nightly","launchOnStartup":false,"transcodeHeight":480}`))
	request.RemoteAddr = "127.0.0.1:50000"
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	if server.config.Get().Port != 9000 {
		t.Fatalf("settings were not saved")
	}
}

func TestProbeImage(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/resource/svg/cross.svg", nil)
	request.RemoteAddr = "127.0.0.1:50000"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("unexpected probe response")
	}
}

func TestAllowedHLSURL(t *testing.T) {
	allowed, _ := url.Parse("https://rr1---sn.example.googlevideo.com/videoplayback/seg.ts")
	if !allowedHLSURL(allowed) {
		t.Fatal("expected googlevideo URL to be allowed")
	}
	for _, raw := range []string{
		"http://rr1.googlevideo.com/seg.ts",
		"https://googlevideo.com.evil.example/seg.ts",
		"https://user@googlevideo.com/seg.ts",
		"https://googlevideo.com:444/seg.ts",
	} {
		value, _ := url.Parse(raw)
		if allowedHLSURL(value) {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}
}

func TestRewriteHLSManifest(t *testing.T) {
	base, _ := url.Parse("https://manifest.googlevideo.com/live/playlist/index.m3u8")
	body := "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\nsegment.ts\nhttps://evil.example/segment.ts\n"
	got := rewriteHLSManifest(body, base, "http://127.0.0.1:8765/api/youtube/hls?url=")
	if !strings.Contains(got, "url=https%3A%2F%2Fmanifest.googlevideo.com%2Flive%2Fplaylist%2Fsegment.ts") {
		t.Fatalf("relative segment was not rewritten: %s", got)
	}
	if !strings.Contains(got, `URI="http://127.0.0.1:8765/api/youtube/hls?url=https%3A%2F%2Fmanifest.googlevideo.com%2Flive%2Fplaylist%2Fkey.bin"`) {
		t.Fatalf("URI attribute was not rewritten: %s", got)
	}
	if !strings.Contains(got, "https://evil.example/segment.ts") {
		t.Fatalf("disallowed URL should remain unchanged: %s", got)
	}
}

func TestHLSProxyRejectsArbitraryHosts(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/youtube/hls?url=https%3A%2F%2Fexample.com%2Fvideo.m3u8", nil)
	request.RemoteAddr = "127.0.0.1:50000"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestTranscodeVideoRatesScaleWithResolution(t *testing.T) {
	if bitrate, maxrate, buffer := transcodeVideoRates(1080); bitrate != "4000k" || maxrate != "5200k" || buffer != "8000k" {
		t.Fatalf("unexpected 1080p rates: %s %s %s", bitrate, maxrate, buffer)
	}
	if bitrate, maxrate, buffer := transcodeVideoRates(360); bitrate != "650k" || maxrate != "850k" || buffer != "1300k" {
		t.Fatalf("unexpected 360p rates: %s %s %s", bitrate, maxrate, buffer)
	}
}

func TestYouTubeHeartbeatCreatesAndReleasesStreamLease(t *testing.T) {
	server := testServer(t)
	sessionID := "test_session_1234567890"
	sendHeartbeat := func(keepAlive bool) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"sessionId":%q,"keepStreamAlive":%t,"protocolMin":2,"protocolMax":2}`, sessionID, keepAlive)
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/youtube/heartbeat", strings.NewReader(body))
		request.RemoteAddr = "127.0.0.1:50000"
		recorder := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(recorder, request)
		return recorder
	}
	if recorder := sendHeartbeat(true); recorder.Code != http.StatusOK {
		t.Fatalf("heartbeat status %d: %s", recorder.Code, recorder.Body.String())
	}
	cancelled := make(chan struct{})
	relayID, ok := server.registerStream(sessionID, func() { close(cancelled) })
	if !ok {
		t.Fatal("active heartbeat did not create a stream lease")
	}
	if recorder := sendHeartbeat(false); recorder.Code != http.StatusOK {
		t.Fatalf("release status %d: %s", recorder.Code, recorder.Body.String())
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("release heartbeat did not cancel the stream")
	}
	server.unregisterStream(sessionID, relayID)
}

func TestYouTubeHeartbeatRejectsInvalidSession(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/youtube/heartbeat", strings.NewReader(`{"sessionId":"short","keepStreamAlive":true}`))
	request.RemoteAddr = "127.0.0.1:50000"
	recorder := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestYouTubeHeartbeatReportsUpgradeDirection(t *testing.T) {
	server := testServer(t)
	for _, test := range []struct {
		body string
		want string
	}{
		{`{"sessionId":"test_session_1234567890","keepStreamAlive":true,"protocolMin":1,"protocolMax":1}`, "wallpaper is too old"},
		{`{"sessionId":"test_session_1234567890","keepStreamAlive":true,"protocolMin":3,"protocolMax":3}`, "companion is too old"},
	} {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/youtube/heartbeat", strings.NewReader(test.body))
		request.RemoteAddr = "127.0.0.1:50000"
		recorder := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUpgradeRequired || !strings.Contains(strings.ToLower(recorder.Body.String()), test.want) {
			t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
		}
	}
}
