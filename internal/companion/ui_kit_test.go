package companion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// The UI kit is a pinned copy of the wallpaper's; only its export tool may change it.
func TestUIKitMatchesItsPin(t *testing.T) {
	var pin struct {
		SourceRepository string
		SourceRevision   string
		Files            map[string]struct{ Source, SHA256 string }
	}
	if err := json.Unmarshal(kitPin, &pin); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(pin.SourceRevision) || pin.SourceRepository == "" {
		t.Fatalf("bad pin: %+v", pin)
	}
	embedded := map[string]string{"zzz_ui.css": kitCSS, "zzz_ui.js": kitJS}
	if len(pin.Files) != len(embedded) {
		t.Fatalf("pin lists %d files, %d are embedded", len(pin.Files), len(embedded))
	}
	for name, text := range embedded {
		sum := sha256.Sum256([]byte(text))
		if hex.EncodeToString(sum[:]) != pin.Files[name].SHA256 {
			t.Errorf("%s differs from the pinned copy; change it in the wallpaper and re-export (ui/zzz/README.md)", name)
		}
	}
}

func TestUIKitIsServed(t *testing.T) {
	s := testServer(t)
	theme := cartRequest(t, s, http.MethodGet, "/ui/theme.css", nil).Body.String()
	if !strings.Contains(theme, kitCSS) || !strings.HasSuffix(theme, kitScale) {
		t.Fatal("theme.css does not carry the kit and its scale")
	}
	r := cartRequest(t, s, http.MethodGet, "/ui/zzz/zzz_ui.js", nil)
	if r.Code != http.StatusOK || r.Body.String() != kitJS || !strings.HasPrefix(r.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("kit script: %d %s", r.Code, r.Header().Get("Content-Type"))
	}
}

// Status lines are message bars from the kit: pages go through companionSay, never straight to the element.
func TestStatusLinesUseTheMessageBar(t *testing.T) {
	s := testServer(t)
	helper := cartRequest(t, s, http.MethodGet, "/ui/api.js", nil).Body.String()
	if !strings.Contains(helper, "window.companionSay") || !strings.Contains(helper, "createMessage") {
		t.Fatal("/ui/api.js does not carry the status helper")
	}
	for _, path := range []string{"/settings", "/dates", "/holidays", "/games"} {
		body := cartRequest(t, s, http.MethodGet, path, nil).Body.String()
		if strings.Contains(body, "status.textContent") || strings.Contains(body, "$('status').textContent") {
			t.Errorf("%s writes its status as plain text", path)
		}
	}
	if strings.Contains(controlsJS, "status.textContent") {
		t.Error("controls.js writes a status as plain text")
	}
}
