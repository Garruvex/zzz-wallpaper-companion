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

//go:embed operations_generated.js
var operationsJS string

//go:embed browser-client.js
var browserClientJS string

//go:embed status.js
var statusJS string

func companionBrowserAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.WriteString(w, operationsJS)
	io.WriteString(w, browserClientJS)
	io.WriteString(w, statusJS)
}

// The shared ZZZ UI kit, copied from the wallpaper by its tools/export-ui-kit.cjs
// and pinned in ui/zzz/kit-pin.json (see ui/zzz/README.md). Never edit it here.
//
//go:embed ui/zzz/zzz_ui.css
var kitCSS string

//go:embed ui/zzz/zzz_ui.js
var kitJS string

//go:embed ui/zzz/kit-pin.json
var kitPin []byte

// The kit is sized for the wallpaper's 1080p screen; these pages scale it to the window width.
const kitScale = ":root{--zzz-u:clamp(.7px,.075vw,1px)}\n"

func companionTheme(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	io.WriteString(w, themeCSS)
	io.WriteString(w, kitCSS)
	io.WriteString(w, kitScale)
}

func companionKit(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	io.WriteString(w, kitJS)
}
func companionShell(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	info, _ := json.Marshal(map[string]any{"version": version, "build": buildNumber, "defaults": defaultSettings()})
	io.WriteString(w, "const companionBuildInfo = "+string(info)+";\n")
	io.WriteString(w, shellJS)
	io.WriteString(w, controlsJS)
}
