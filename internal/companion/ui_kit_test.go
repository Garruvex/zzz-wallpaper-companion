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

// The browser's own validation bubbles would swallow a bad entry before the page can say so in a
// message bar, so these forms check for themselves.
func TestFormsReportProblemsInTheMessageBar(t *testing.T) {
	s := testServer(t)
	for path, form := range map[string]string{"/settings": `<form id="settings" novalidate>`, "/dates": `<form id="dateForm" novalidate>`} {
		if !strings.Contains(cartRequest(t, s, http.MethodGet, path, nil).Body.String(), form) {
			t.Errorf("%s: the form lets the browser swallow its own errors", path)
		}
	}
}

// Three tabs along the top right and a side menu for each: Calendar lists its custom dates and
// holidays pages, General and Game the sections of their page.
// The theme keeps to the game's rules: nothing reacts to the mouse resting on it, and the accents
// are lime and green only.
func TestShellHasThreeTopTabsAndNoHoverOrYellow(t *testing.T) {
	for _, want := range []string{"'General'", "'Calendar'", "'Game'", "'Custom Date'", "'Holiday'", "'/dates'", "'/holidays'", "'Startup & Updates'", "'Library'", "'Add Games'"} {
		if !strings.Contains(shellJS, want) {
			t.Errorf("shell.js lacks %s", want)
		}
	}
	for _, want := range []string{"top-tabs", "no-side"} {
		if !strings.Contains(shellJS, want) || !strings.Contains(themeCSS, want) {
			t.Errorf("the top tabs or the no-side layout is missing: %s", want)
		}
	}
	for _, old := range []string{"'Important Dates'", "'Holidays']", "subtabs"} {
		if strings.Contains(shellJS, old) {
			t.Errorf("shell.js still lists the old tab %s", old)
		}
	}
	for _, rule := range []string{":hover", "#ffe500", "#e7f000", "#fff04b", "#f3c942"} {
		if strings.Contains(themeCSS, rule) || strings.Contains(gamesHTML, rule) {
			t.Errorf("theme or Games page still has %s", rule)
		}
	}
}

// Panels are built like the game's item cards (a black title bar over
// a body) for settings, forms, saved dates, holiday months and games.
func TestPanelsUseTitleAndItemCards(t *testing.T) {
	s := testServer(t)
	helper := cartRequest(t, s, http.MethodGet, "/ui/api.js", nil).Body.String()
	for _, want := range []string{"window.companionTitleCard", "window.companionCard"} {
		if !strings.Contains(helper, want) {
			t.Errorf("/ui/api.js lacks %s", want)
		}
	}
	for _, want := range []string{"companionCard(", "zzz-item-bar"} {
		if !strings.Contains(shellJS, want) {
			t.Errorf("shell.js lacks %s", want)
		}
	}
	for path, page := range map[string]string{"dates": datesHTML} {
		if !strings.Contains(page, "companionCard(document") {
			t.Errorf("%s does not render its list as item cards", path)
		}
	}
	// The holidays are a list of dates, not slots: date blocks under Coming up and Earlier, and not the
	// shelf's tape rows.
	if !strings.Contains(holidaysHTML, "date-block") || !strings.Contains(holidaysHTML, "Coming up") || strings.Contains(holidaysHTML, "tape-row") {
		t.Error("the holidays are not listed as dates")
	}
	// The Library is the shelf as a list, 01 to 14, numbered like the tapes.
	if !strings.Contains(gamesHTML, "tape-num") || !strings.Contains(gamesHTML, "tape <= 14") {
		t.Error("the Library is not listed by tape")
	}
	// The number tile moves a game to another tape, and a section is not titled again over its cards.
	if !strings.Contains(gamesHTML, "tapePicker(") || !strings.Contains(gamesHTML, "action: 'tape'") {
		t.Error("the tape number cannot be changed")
	}
	if strings.Contains(shellJS, "companionTitleCard(") {
		t.Error("the shell puts a repeated title card over each section")
	}
	if strings.Contains(gamesHTML, "library-columns") {
		t.Error("the Games page still has the table headings")
	}
}

// On/off settings are the game's ON / OFF switch, not an Enable / Disable dropdown.
func TestCheckboxesAreSwitches(t *testing.T) {
	if !strings.Contains(controlsJS, "function toggle(input)") || !strings.Contains(controlsJS, "querySelectorAll('input[type=checkbox]').forEach(toggle)") {
		t.Error("checkboxes are not turned into switches")
	}
	if strings.Contains(controlsJS, "querySelectorAll('select,input[type=checkbox]')") {
		t.Error("checkboxes still go through the dropdown")
	}
	if !strings.Contains(themeCSS, ".zzz-switch{") {
		t.Error("the theme lacks the switch")
	}
}

// Small buttons are the kit's chip button, and anything that asks goes through the kit's Confirm /
// Cancel dialog (companionConfirm), never the browser's own confirm().
func TestSmallButtonsAndConfirmUseTheKit(t *testing.T) {
	s := testServer(t)
	if helper := cartRequest(t, s, http.MethodGet, "/ui/api.js", nil).Body.String(); !strings.Contains(helper, "window.companionConfirm") || !strings.Contains(helper, "confirmDialog") {
		t.Error("/ui/api.js lacks the confirm helper")
	}
	for name, page := range map[string]string{"games": gamesHTML, "dates": datesHTML, "holidays": holidaysHTML} {
		if strings.Contains(page, "confirm(") && !strings.Contains(page, "companionConfirm(") {
			t.Errorf("%s asks with the browser's confirm()", name)
		}
		for _, line := range strings.Split(page, "\n") {
			if strings.Contains(line, "confirm(") && !strings.Contains(line, "companionConfirm(") && !strings.Contains(line, "window.confirm") {
				t.Errorf("%s still has a browser confirm(): %.80s", name, line)
			}
		}
	}
	if !strings.Contains(gamesHTML, "zzz-chipbtn") || !strings.Contains(datesHTML, "zzz-chipbtn") {
		t.Error("the Games or Dates buttons are not the kit's chip button")
	}
}

// The Controller section of the Game tab reads the pad through a page helper, not the public API, and
// shows only buttons and sticks.
func TestControllerSectionShowsThePad(t *testing.T) {
	if !strings.Contains(shellJS, "['controller', 'Controller']") || !strings.Contains(gamesHTML, "companionPadState") {
		t.Fatal("the Game tab has no Controller section")
	}
	s := testServer(t)
	r := cartRequest(t, s, http.MethodGet, "/controller/state", nil)
	if r.Code != http.StatusOK {
		t.Fatalf("controller state: %d", r.Code)
	}
	var state struct {
		Connected bool
		Buttons   uint32
		Axes      [4]int16
	}
	if err := json.Unmarshal(r.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
}
