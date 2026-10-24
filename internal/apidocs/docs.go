//go:build apidocs

// Package apidocs serves bundled documentation without network dependencies.
// It is only built with -tags apidocs, so release builds leave the explorer out.
package apidocs

import (
	"embed"
	"github.com/Garruvex/zzz-wallpaper-companion/api"
	"io"
	"net/http"
)

//go:embed assets/*
var assets embed.FS

// Enabled reports whether this build serves the explorer.
const Enabled = true

func Routes() map[string]http.Handler {
	return map[string]http.Handler{
		"GET /openapi.json": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write(api.Contract)
		}),
		"GET /docs": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serve(w, r, "index.html", "text/html; charset=utf-8")
		}),
		"GET /docs/{asset}": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			name := r.PathValue("asset")
			types := map[string]string{"index.html": "text/html; charset=utf-8", "explorer.js": "text/javascript; charset=utf-8", "swagger-ui-bundle.js": "text/javascript; charset=utf-8", "swagger-ui.css": "text/css; charset=utf-8", "LICENSE": "text/plain; charset=utf-8", "NOTICE": "text/plain; charset=utf-8", "vendor.json": "application/json; charset=utf-8"}
			typ, ok := types[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			serve(w, r, name, typ)
		}),
	}
}

func Register(mux *http.ServeMux) {
	for pattern, handler := range Routes() {
		mux.Handle(pattern, handler)
	}
}

func serve(w http.ResponseWriter, r *http.Request, name, typ string) {
	f, err := assets.Open("assets/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", typ)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, f)
}
