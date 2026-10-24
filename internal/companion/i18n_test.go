package companion

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var placeholder = regexp.MustCompile(`\{\w+\}`)

func placeholders(text string) string {
	found := placeholder.FindAllString(text, -1)
	sort.Strings(found)
	return strings.Join(found, ",")
}

// setLanguage makes the companion speak lang for the test, then goes back to English.
func setLanguage(t *testing.T, dir, lang string) {
	t.Helper()
	languageMu.Lock()
	languageDir, languageChoice = dir, lang
	languageMu.Unlock()
	t.Cleanup(func() {
		languageMu.Lock()
		languageDir, languageChoice = "", ""
		languageMu.Unlock()
	})
}

func TestEveryStringHasBothChineseVersionsWithTheSamePlaceholders(t *testing.T) {
	for english, versions := range translations() {
		for _, lang := range []string{"zh-CN", "zh-TW"} {
			text := versions[lang]
			if text == "" {
				t.Errorf("%q has no %s version", english, lang)
				continue
			}
			if placeholders(text) != placeholders(english) {
				t.Errorf("%q: %s version %q has different {placeholders}", english, lang, text)
			}
		}
		if len(versions) != 2 {
			t.Errorf("%q has an unexpected language: %v", english, versions)
		}
	}
}

// What the code asks to be translated must be in the table, or the page shows English to Chinese readers.
func TestEveryStringTheCodeTranslatesIsInTheTable(t *testing.T) {
	table := translations()
	unquote := func(raw string) string {
		raw = strings.ReplaceAll(raw, `\'`, `'`)
		raw = strings.ReplaceAll(raw, `\"`, `"`)
		return raw
	}
	// Pages: t('…') calls, and strings handed to companionSay / textContent / titles that are plain English.
	call := regexp.MustCompile(`(?:[^\w.$]|^)t\(\s*'((?:\\.|[^'\\])*)'`)
	// Go: T("…"), Tf("…", …) and writeError(w, status, "…").
	goCall := regexp.MustCompile(`(?:\bT|\bTf)\(\s*"((?:\\.|[^"\\])*)"|writeError\(w, http\.Status\w+, "((?:\\.|[^"\\])*)"`)
	files, _ := filepath.Glob("*")
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		text := string(data)
		var wanted []string
		text = withoutComments(text, name)
		switch {
		case strings.HasSuffix(name, ".html"), strings.HasSuffix(name, ".js"):
			for _, m := range call.FindAllStringSubmatch(text, -1) {
				wanted = append(wanted, unquote(m[1]))
			}
		case strings.HasSuffix(name, ".go"):
			for _, m := range goCall.FindAllStringSubmatch(text, -1) {
				wanted = append(wanted, unquote(m[1]+m[2]))
			}
		}
		for _, english := range wanted {
			if english == "" || !strings.ContainsAny(english, "abcdefghijklmnopqrstuvwxyz") {
				continue
			}
			checked++
			if _, ok := table[english]; !ok && !technicalError(english) {
				t.Errorf("%s asks to translate %q, which is not in i18n.json", name, english)
			}
		}
	}
	if checked < 100 {
		t.Fatalf("only %d strings found; the scan is not reading the sources", checked)
	}
}

// withoutComments blanks whole-line comments, which show examples such as t('… {name} …').
func withoutComments(text, name string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// Errors only the wallpaper's own pages and the YouTube relay produce stay English on purpose.
func technicalError(text string) bool {
	for _, prefix := range []string{"FFmpeg", "HLS", "invalid ", "map provider", "cannot request", "capture must", "streaming ", "stream session", "only save", "unknown action", "no such cartridge", "not there yet", "could not write file", "PSP games are locked", "expected "} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func TestTranslateFallsBackToEnglishAndFillsPlaceholders(t *testing.T) {
	if got := translateTo("zh-CN", "Quit"); got != "退出" {
		t.Fatalf("Quit in zh-CN: %q", got)
	}
	if got := translateTo("zh-TW", "Quit"); got != "結束" {
		t.Fatalf("Quit in zh-TW: %q", got)
	}
	if got := translateTo("zh-CN", "A string nobody translated"); got != "A string nobody translated" {
		t.Fatalf("a missing string should stay English: %q", got)
	}
	if got := translateTo("en", "Quit"); got != "Quit" {
		t.Fatalf("English: %q", got)
	}
	setLanguage(t, t.TempDir(), "zh-TW")
	if got := Tf("Version {version} is the latest compatible release.", "version", "1.4.0"); got != "版本 1.4.0 是最新的相容版本。" {
		t.Fatalf("Tf: %q", got)
	}
}

func TestLanguageChoiceIsSavedAndDrivesThePagesAndErrors(t *testing.T) {
	server := testServer(t)
	loadLanguage(server.dataDir)
	t.Cleanup(func() { loadLanguage("") })
	setLanguage(t, server.dataDir, "")

	var got struct{ Language, Current string }
	decode(t, cartRequest(t, server, http.MethodGet, "/ui/language", nil).Body.Bytes(), &got)
	if got.Language != "" || got.Current != "en" {
		t.Fatalf("a fresh companion should follow Windows (English in tests): %+v", got)
	}

	if r := cartRequest(t, server, http.MethodPost, "/ui/language", []byte(`{"language":"fr"}`)); r.Code != http.StatusBadRequest {
		t.Fatalf("an unknown language answered %d", r.Code)
	}
	if r := cartRequest(t, server, http.MethodPost, "/ui/language", []byte(`{"language":"zh-TW"}`)); r.Code != http.StatusOK {
		t.Fatalf("saving the language answered %d %s", r.Code, r.Body)
	}
	if data, err := os.ReadFile(filepath.Join(server.dataDir, languageFile)); err != nil || !strings.Contains(string(data), "zh-TW") {
		t.Fatalf("language.json: %q %v", data, err)
	}

	// The pages get the table first thing in api.js.
	api := cartRequest(t, server, http.MethodGet, "/ui/api.js", nil).Body.String()
	var table struct {
		Lang  string            `json:"lang"`
		Table map[string]string `json:"table"`
	}
	line := api[:strings.Index(api, "\n")]
	if err := json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(line, "window.companionI18n="), ";")), &table); err != nil {
		t.Fatalf("api.js does not start with the table: %v", err)
	}
	if table.Lang != "zh-TW" || table.Table["Storage"] != "儲存空間" {
		t.Fatalf("table: %+v", table.Lang)
	}

	// Choosing a language answers with its table, so open pages switch without a reload.
	var picked struct {
		Current string            `json:"current"`
		Table   map[string]string `json:"table"`
	}
	decode(t, cartRequest(t, server, http.MethodGet, "/ui/language", nil).Body.Bytes(), &picked)
	if picked.Current != "zh-TW" || picked.Table["Quit"] != "結束" {
		t.Fatalf("the language answer lacks its table: %q, %d entries", picked.Current, len(picked.Table))
	}

	// The errors a page shows come back in the language too.
	r := cartRequest(t, server, http.MethodPost, "/games/rename", []byte(`{"id":"Nope","title":"x"}`))
	var failure struct{ Error string }
	decode(t, r.Body.Bytes(), &failure)
	if failure.Error != "該遊戲已不在遊戲庫中。" {
		t.Fatalf("error text: %q", failure.Error)
	}

	// Choosing "Windows language" again clears the choice.
	cartRequest(t, server, http.MethodPost, "/ui/language", []byte(`{"language":""}`))
	decode(t, cartRequest(t, server, http.MethodGet, "/ui/language", nil).Body.Bytes(), &got)
	if got.Language != "" {
		t.Fatalf("the choice was not cleared: %+v", got)
	}

	// A saved choice survives a restart.
	cartRequest(t, server, http.MethodPost, "/ui/language", []byte(`{"language":"zh-CN"}`))
	loadLanguage(server.dataDir)
	if currentLanguage() != "zh-CN" {
		t.Fatalf("after restart: %s", currentLanguage())
	}
}
