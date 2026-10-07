package companion

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func listenerPort(t *testing.T, address string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	var value int
	if _, err := fmt.Sscan(port, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestCompanionRespondingChecksIdentityAndRejectsRedirects(t *testing.T) {
	for _, tc := range []struct {
		body     string
		redirect bool
		want     bool
	}{
		{`{"name":"zzz-wallpaper-companion","protocolVersion":2,"ready":false}`, false, true},
		{`{"name":"another-app","protocolVersion":2}`, false, false},
		{`{"name":"zzz-wallpaper-companion"}`, false, false},
		{`not json`, false, false},
		{``, true, false},
	} {
		t.Run(tc.body+fmt.Sprint(tc.redirect), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.redirect {
					http.Redirect(w, r, "http://example.invalid", http.StatusFound)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			if got := companionResponding(listenerPort(t, strings.TrimPrefix(server.URL, "http://"))); got != tc.want {
				t.Fatalf("responding=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestOccupiedPortKeepsSettingsAvailableWithoutMovingWallpaperServices(t *testing.T) {
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	server := testServer(t)
	settings := server.config.Get()
	settings.Port = occupied.Addr().(*net.TCPAddr).Port
	if err := server.config.Save(settings); err != nil {
		t.Fatal(err)
	}
	listener, warning, err := server.prepareListener()
	if err != nil {
		t.Fatal(err)
	}
	if warning == "" || listener.Addr().(*net.TCPAddr).Port == settings.Port || server.config.Get().Port != settings.Port {
		t.Fatal("missing recovery or configured port changed")
	}
	go server.http.Serve(listener)
	defer server.Shutdown(context.Background())
	base := "http://" + listener.Addr().String()
	response, err := http.Get(base + "/settings")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || !strings.Contains(string(body), "Connection unavailable") || !strings.Contains(string(body), "Task Manager") || !strings.Contains(string(body), "8765") {
		t.Fatalf("missing recovery instructions: %s", body)
	}
	response, err = http.Get(base + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatal("recovery must not silently expose wallpaper services")
	}
	settings.Port = 9001
	payload := fmt.Sprintf(`{"port":%d,"maxHeight":%d,"updateChannel":%q,"launchOnStartup":%t,"transcodeHeight":%d,"autoUpdate":%t}`, settings.Port, settings.MaxHeight, settings.UpdateChannel, settings.LaunchOnStartup, settings.TranscodeHeight, settings.AutoUpdate)
	response, err = http.Post(base+"/api/settings", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || server.config.Get().Port != 9001 || !strings.Contains(string(body), `"restartRequired":true`) {
		t.Fatalf("could not recover settings: %s", body)
	}
}

func TestFreePortStartsNormalAPI(t *testing.T) {
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()
	server := testServer(t)
	settings := server.config.Get()
	settings.Port = port
	if err := server.config.Save(settings); err != nil {
		t.Fatal(err)
	}
	listener, warning, err := server.prepareListener()
	if err != nil {
		t.Fatal(err)
	}
	if warning != "" {
		t.Fatal(warning)
	}
	go server.http.Serve(listener)
	defer server.Shutdown(context.Background())
	if !companionResponding(port) {
		t.Fatal("normal API is unavailable")
	}
}
