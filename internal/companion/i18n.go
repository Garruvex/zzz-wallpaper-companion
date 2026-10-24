package companion

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The companion speaks English, Simplified Chinese or Traditional Chinese. Text is written in English
// everywhere and i18n.json maps each English string to its two Chinese versions, so a string missing
// from the table simply stays English. The pages get the current language's table in /ui/api.js and
// translate themselves (status.js); the Go side uses T for the tray menu and the errors pages show.
//
// The language follows Windows' display language until one is chosen in General → Language, which is
// kept in language.json in the data folder. It is a page setting, not part of the wallpaper's API.

//go:embed i18n.json
var i18nJSON []byte

const languageFile = "language.json"

// Languages the companion can show, in the order the picker lists them.
var languages = []string{"en", "zh-CN", "zh-TW"}

var (
	i18nOnce  sync.Once
	i18nTable map[string]map[string]string // English → language → text

	languageMu     sync.RWMutex
	languageDir    string // where language.json lives; empty until loadLanguage (tests stay in English)
	languageChoice string // "" follows Windows
)

func translations() map[string]map[string]string {
	i18nOnce.Do(func() {
		if err := json.Unmarshal(i18nJSON, &i18nTable); err != nil {
			panic("i18n.json: " + err.Error())
		}
	})
	return i18nTable
}

func validLanguage(lang string) bool {
	for _, l := range languages {
		if l == lang {
			return true
		}
	}
	return false
}

// loadLanguage reads the chosen language from the data folder at startup.
func loadLanguage(dataDir string) {
	languageMu.Lock()
	defer languageMu.Unlock()
	languageDir = dataDir
	languageChoice = ""
	var saved struct {
		Language string `json:"language"`
	}
	if data, err := os.ReadFile(filepath.Join(dataDir, languageFile)); err == nil && json.Unmarshal(data, &saved) == nil && validLanguage(saved.Language) {
		languageChoice = saved.Language
	}
}

// currentLanguage is the language the companion shows now.
func currentLanguage() string {
	languageMu.RLock()
	defer languageMu.RUnlock()
	if languageDir == "" {
		return "en"
	}
	if languageChoice != "" {
		return languageChoice
	}
	return systemLanguage()
}

// T translates English text into the current language.
func T(text string) string { return translateTo(currentLanguage(), text) }

// Tf translates text with {name} placeholders, then fills them: Tf("Hi {who}", "who", name).
func Tf(text string, pairs ...string) string {
	out := T(text)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = strings.ReplaceAll(out, "{"+pairs[i]+"}", pairs[i+1])
	}
	return out
}

func translateTo(lang, text string) string {
	if lang == "en" {
		return text
	}
	if versions, ok := translations()[text]; ok && versions[lang] != "" {
		return versions[lang]
	}
	return text
}

// tableFor is the English → text map the pages translate with.
func tableFor(lang string) map[string]string {
	table := map[string]string{}
	if lang == "en" {
		return table
	}
	for english, versions := range translations() {
		if text := versions[lang]; text != "" {
			table[english] = text
		}
	}
	return table
}

func (s *APIServer) getLanguage(w http.ResponseWriter, _ *http.Request) {
	languageMu.RLock()
	choice := languageChoice
	languageMu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{"language": choice, "current": currentLanguage(), "system": systemLanguage(), "table": tableFor(currentLanguage())})
}

// saveLanguage stores the choice ("" follows Windows again). The page reloads to show it.
func (s *APIServer) saveLanguage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Language string `json:"language"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || (body.Language != "" && !validLanguage(body.Language)) {
		writeError(w, http.StatusBadRequest, "Unknown language.")
		return
	}
	languageMu.Lock()
	dir := languageDir
	if dir == "" {
		dir = s.dataDir
		languageDir = dir
	}
	data, _ := json.Marshal(map[string]string{"language": body.Language})
	err := writeFileAtomic(filepath.Join(dir, languageFile), data)
	if err == nil {
		languageChoice = body.Language
	}
	languageMu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save the language.")
		return
	}
	s.getLanguage(w, r)
}
