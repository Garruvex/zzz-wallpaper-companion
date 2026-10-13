package companion

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
)

//go:embed theme.css
var themeCSS string

//go:embed shell.js
var shellJS string

//go:embed controls.js
var controlsJS string

//go:embed pills.css
var pillsCSS string

func companionTheme(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	io.WriteString(w, themeCSS)
	io.WriteString(w, pillsCSS)
}
func companionShell(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	info, _ := json.Marshal(map[string]any{"version": version, "build": buildNumber, "defaults": defaultSettings()})
	io.WriteString(w, "const companionBuildInfo = "+string(info)+";\n")
	io.WriteString(w, shellJS)
	io.WriteString(w, controlsJS)
}
