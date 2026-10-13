package companion

import (
	_ "embed"
	"io"
	"net/http"
)

//go:embed theme.css
var themeCSS string

//go:embed shell.js
var shellJS string

func companionTheme(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	io.WriteString(w, themeCSS)
}
func companionShell(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	io.WriteString(w, shellJS)
}
