package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const protocolVersion = 1

var youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,64}$`)

type APIServer struct {
	config   *ConfigStore
	resolver *Resolver
	http     *http.Server
}

func newAPIServer(config *ConfigStore, resolver *Resolver) *APIServer {
	s := &APIServer{config: config, resolver: resolver}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/youtube/resolve", s.resolve)
	mux.HandleFunc("GET /api/youtube/playlist", s.playlist)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("GET /resource/svg/cross.svg", s.probeImage)
	mux.HandleFunc("GET /settings", s.settingsPage)
	mux.HandleFunc("GET /", s.root)
	s.http = &http.Server{
		Handler:           localOnly(cors(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s
}

func (s *APIServer) ListenAndServe() error {
	port := s.config.Get().Port
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("port %d is unavailable: %w", port, err)
	}
	return s.http.Serve(listener)
}

func (s *APIServer) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *APIServer) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ready": true, "name": "zzz-wallpaper-companion", "protocolVersion": protocolVersion,
	})
}

func (s *APIServer) probeImage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><path d="M0 0h1v1H0z"/></svg>`))
}

func (s *APIServer) resolve(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validYouTubeID(id) {
		writeError(w, http.StatusBadRequest, "invalid YouTube video ID")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	result, err := s.resolver.Resolve(ctx, id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *APIServer) playlist(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validYouTubeID(id) {
		writeError(w, http.StatusBadRequest, "invalid YouTube playlist ID")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	items, err := s.resolver.Playlist(ctx, id)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "items": items})
}

func (s *APIServer) getSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.config.Get())
}

func (s *APIServer) saveSettings(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	var next Settings
	if err := decoder.Decode(&next); err != nil {
		writeError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
		return
	}
	previous := s.config.Get()
	if err := s.config.Save(next); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved": true, "restartRequired": previous.Port != next.Port,
	})
}

func (s *APIServer) settingsPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = settingsTemplate.Execute(w, s.config.Get())
}

func (s *APIServer) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusTemporaryRedirect)
}

func validYouTubeID(value string) bool { return youtubeIDPattern.MatchString(value) }

func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			http.Error(w, "loopback access only", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin != "null" && !strings.HasPrefix(origin, "http://127.0.0.1:") && !strings.HasPrefix(origin, "http://localhost:") {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

var settingsTemplate = template.Must(template.New("settings").Funcs(template.FuncMap{
	"heights": func() []int { return []int{360, 480, 720, 1080} },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>ZZZ Wallpaper Companion</title><style>
:root{color-scheme:dark;font-family:Segoe UI,Arial,sans-serif;background:#111315;color:#f3f4f4}body{margin:0}.bar{height:5px;background:#f3c942}.wrap{max-width:620px;margin:0 auto;padding:36px 22px}h1{font-size:25px;margin:0 0 7px}.sub{color:#aeb5b8;margin:0 0 30px}.section{border-top:1px solid #34383a;padding:22px 0}label{display:grid;grid-template-columns:1fr 190px;gap:18px;align-items:center;margin:0 0 18px}small{display:block;color:#949b9e;margin-top:4px}input,select{box-sizing:border-box;width:100%;background:#1c2022;color:#fff;border:1px solid #4a5053;padding:9px;border-radius:4px;font:inherit}button{background:#f3c942;color:#171717;border:0;padding:10px 18px;border-radius:4px;font-weight:700;cursor:pointer}.status{margin-left:12px;color:#aeb5b8}@media(max-width:520px){label{grid-template-columns:1fr}}
</style></head><body><div class="bar"></div><main class="wrap"><h1>ZZZ Wallpaper Companion</h1><p class="sub">Local playback service</p><form id="settings"><div class="section">
<label><span>Companion port<small>Match this value in Wallpaper Engine. Restart required.</small></span><input id="port" type="number" min="1024" max="65535" value="{{.Port}}"></label>
<label><span>Maximum resolution<small>Higher resolutions use more bandwidth and GPU memory.</small></span><select id="height">{{range $v := heights}}<option>{{$v}}</option>{{end}}</select></label>
<label><span>yt-dlp channel<small>Nightly is recommended for timely YouTube fixes.</small></span><select id="channel"><option value="nightly">Nightly</option><option value="stable">Stable</option></select></label>
</div><button type="submit">Save settings</button><span class="status" id="status"></span></form></main><script>
const initial={port:{{.Port}},maxHeight:{{.MaxHeight}},updateChannel:{{printf "%q" .UpdateChannel}}};
height.value=initial.maxHeight;channel.value=initial.updateChannel;
settings.addEventListener('submit',async e=>{e.preventDefault();status.textContent='Saving...';try{const r=await fetch('/api/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({port:Number(port.value),maxHeight:Number(height.value),updateChannel:channel.value})});const j=await r.json();if(!r.ok)throw new Error(j.error);status.textContent=j.restartRequired?'Saved. Restart the companion.':'Saved.'}catch(e){status.textContent=e.message}});
</script></body></html>`))
