package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	protocolVersion       = 2
	protocolMinVersion    = 2
	protocolMaxVersion    = 2
	streamLeaseDuration   = 20 * time.Second
	streamLeaseCheckEvery = time.Second
)

var youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,64}$`)
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
var hlsURIAttributePattern = regexp.MustCompile(`URI="([^"]+)"`)

type streamSession struct {
	expiresAt time.Time
	keepAlive bool
	cancel    context.CancelFunc
	relayID   uint64
}

type APIServer struct {
	config      *ConfigStore
	resolver    *Resolver
	ffmpeg      *FFmpegManager
	http        *http.Server
	sessionsMu  sync.Mutex
	sessions    map[string]*streamSession
	nextRelayID uint64
}

func newAPIServer(config *ConfigStore, resolver *Resolver, ffmpeg *FFmpegManager) *APIServer {
	s := &APIServer{config: config, resolver: resolver, ffmpeg: ffmpeg, sessions: make(map[string]*streamSession)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/youtube/resolve", s.resolve)
	mux.HandleFunc("GET /api/youtube/playlist", s.playlist)
	mux.HandleFunc("GET /api/youtube/hls", s.hlsProxy)
	mux.HandleFunc("GET /api/youtube/transcode", s.transcode)
	mux.HandleFunc("POST /api/youtube/heartbeat", s.youtubeHeartbeat)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("GET /resource/svg/cross.svg", s.probeImage)
	mux.HandleFunc("GET /settings", s.settingsPage)
	mux.HandleFunc("GET /", s.root)
	s.http = &http.Server{
		Handler:           localOnly(cors(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Live WebM responses remain open for the duration of a broadcast.
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

func (s *APIServer) ListenAndServe(onReady ...func()) error {
	port := s.config.Get().Port
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return fmt.Errorf("port %d is unavailable: %w", port, err)
	}
	if len(onReady) > 0 && onReady[0] != nil {
		onReady[0]()
	}
	return s.http.Serve(listener)
}

func (s *APIServer) Shutdown(ctx context.Context) error {
	s.cancelAllStreams()
	return s.http.Shutdown(ctx)
}

func (s *APIServer) youtubeHeartbeat(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var body struct {
		SessionID        string `json:"sessionId"`
		KeepStreamAlive  bool   `json:"keepStreamAlive"`
		ProtocolMin      int    `json:"protocolMin"`
		ProtocolMax      int    `json:"protocolMax"`
		WallpaperVersion string `json:"wallpaperVersion"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || !sessionIDPattern.MatchString(body.SessionID) {
		writeError(w, http.StatusBadRequest, "invalid heartbeat")
		return
	}
	if body.ProtocolMin <= 0 || body.ProtocolMax < body.ProtocolMin || body.ProtocolMax < protocolMinVersion || body.ProtocolMin > protocolMaxVersion {
		direction := "The wallpaper and companion use incompatible protocols. Update both components."
		if body.ProtocolMax > 0 && body.ProtocolMax < protocolMinVersion {
			direction = "The wallpaper is too old for this companion. Update the wallpaper."
		} else if body.ProtocolMin > protocolMaxVersion {
			direction = "The companion is too old for this wallpaper. Update the companion."
		}
		writeJSON(w, http.StatusUpgradeRequired, map[string]any{
			"compatible": false, "error": direction, "companionVersion": version,
			"protocolMin": protocolMinVersion, "protocolMax": protocolMaxVersion,
		})
		return
	}
	now := time.Now()
	var cancel context.CancelFunc
	s.sessionsMu.Lock()
	for id, session := range s.sessions {
		if now.After(session.expiresAt) && session.cancel == nil {
			delete(s.sessions, id)
		}
	}
	session := s.sessions[body.SessionID]
	if session == nil {
		session = &streamSession{}
		s.sessions[body.SessionID] = session
	}
	session.expiresAt = now.Add(streamLeaseDuration)
	session.keepAlive = body.KeepStreamAlive
	streamActive := session.cancel != nil
	if !body.KeepStreamAlive && session.cancel != nil {
		cancel = session.cancel
		session.cancel = nil
		streamActive = false
	}
	expiresAt := session.expiresAt.UnixMilli()
	s.sessionsMu.Unlock()
	if cancel != nil {
		cancel()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ready": true, "compatible": true, "leaseUntil": expiresAt, "streamActive": streamActive,
		"companionVersion": version, "protocolMin": protocolMinVersion, "protocolMax": protocolMaxVersion,
	})
}

func (s *APIServer) registerStream(sessionID string, cancel context.CancelFunc) (uint64, bool) {
	now := time.Now()
	var previous context.CancelFunc
	s.sessionsMu.Lock()
	session := s.sessions[sessionID]
	if session == nil || !session.keepAlive || now.After(session.expiresAt) {
		s.sessionsMu.Unlock()
		return 0, false
	}
	previous = session.cancel
	s.nextRelayID++
	relayID := s.nextRelayID
	session.cancel = cancel
	session.relayID = relayID
	s.sessionsMu.Unlock()
	if previous != nil {
		previous()
	}
	return relayID, true
}

func (s *APIServer) unregisterStream(sessionID string, relayID uint64) {
	s.sessionsMu.Lock()
	if session := s.sessions[sessionID]; session != nil && session.relayID == relayID {
		session.cancel = nil
		session.relayID = 0
	}
	s.sessionsMu.Unlock()
}

func (s *APIServer) streamLeaseAlive(sessionID string) bool {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	session := s.sessions[sessionID]
	return session != nil && session.keepAlive && time.Now().Before(session.expiresAt)
}

func (s *APIServer) cancelAllStreams() {
	s.sessionsMu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.sessions))
	for _, session := range s.sessions {
		if session.cancel != nil {
			cancels = append(cancels, session.cancel)
		}
	}
	s.sessions = make(map[string]*streamSession)
	s.sessionsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *APIServer) health(w http.ResponseWriter, _ *http.Request) {
	ffmpegReady := s.ffmpeg.Ready()
	writeJSON(w, http.StatusOK, map[string]any{
		"ready": ffmpegReady, "name": "zzz-wallpaper-companion", "protocolVersion": protocolVersion,
		"version": version, "protocolMin": protocolMinVersion, "protocolMax": protocolMaxVersion,
		"ytDlpReady": s.resolver.Ready(), "ffmpegReady": ffmpegReady,
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
	compatibilityMode := r.URL.Query().Get("hlsCompat") == "1"
	result, err := s.resolver.Resolve(ctx, id, compatibilityMode)
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

func (s *APIServer) hlsProxy(w http.ResponseWriter, r *http.Request) {
	upstream, err := url.Parse(r.URL.Query().Get("url"))
	if err != nil || !allowedHLSURL(upstream) {
		writeError(w, http.StatusBadRequest, "invalid HLS resource URL")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid HLS resource URL")
		return
	}
	if value := r.Header.Get("Range"); value != "" {
		request.Header.Set("Range", value)
	}
	client := *s.resolver.httpClient
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if !allowedHLSURL(req.URL) {
			return errors.New("HLS redirect host is not allowed")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		writeError(w, http.StatusBadGateway, "HLS resource request failed")
		return
	}
	defer response.Body.Close()
	for _, header := range []string{"Accept-Ranges", "Content-Range"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	contentType := response.Header.Get("Content-Type")
	isManifest := strings.Contains(strings.ToLower(contentType), "mpegurl") || strings.HasSuffix(strings.ToLower(upstream.Path), ".m3u8")
	if !isManifest {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "HLS manifest could not be read")
		return
	}
	proxyBase := fmt.Sprintf("http://127.0.0.1:%d/api/youtube/hls?url=", s.config.Get().Port)
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write([]byte(rewriteHLSManifest(string(body), upstream, proxyBase)))
}

func (s *APIServer) transcode(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !validYouTubeID(id) {
		writeError(w, http.StatusBadRequest, "invalid YouTube video ID")
		return
	}
	sessionID := r.URL.Query().Get("session")
	if !sessionIDPattern.MatchString(sessionID) {
		writeError(w, http.StatusBadRequest, "invalid stream session")
		return
	}
	streamContext, cancelStream := context.WithCancel(r.Context())
	relayID, registered := s.registerStream(sessionID, cancelStream)
	if !registered {
		cancelStream()
		writeError(w, http.StatusConflict, "stream session lease is not active")
		return
	}
	defer func() {
		cancelStream()
		s.unregisterStream(sessionID, relayID)
	}()
	go func() {
		ticker := time.NewTicker(streamLeaseCheckEvery)
		defer ticker.Stop()
		for {
			select {
			case <-streamContext.Done():
				return
			case <-ticker.C:
				if !s.streamLeaseAlive(sessionID) {
					cancelStream()
					return
				}
			}
		}
	}()
	ffmpegPath := s.ffmpeg.path
	if _, err := os.Stat(ffmpegPath); err != nil {
		writeError(w, http.StatusServiceUnavailable, "FFmpeg is not installed")
		return
	}
	media, err := s.resolver.Resolve(streamContext, id, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	height := s.config.Get().TranscodeHeight
	bitrate, maxrate, bufferSize := transcodeVideoRates(height)
	cmd := exec.CommandContext(streamContext, ffmpegPath,
		"-hide_banner", "-loglevel", "warning",
		"-fflags", "+discardcorrupt",
		"-reconnect", "1", "-reconnect_streamed", "1",
		"-reconnect_delay_max", "5", "-rw_timeout", "15000000",
		"-i", media.VideoURL,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-vf", fmt.Sprintf("scale=-2:%d", height),
		"-c:v", "libvpx", "-deadline", "realtime", "-cpu-used", "8",
		"-b:v", bitrate, "-maxrate", maxrate, "-bufsize", bufferSize, "-g", "60",
		"-c:a", "libopus", "-b:a", "64k",
		"-f", "webm", "-live", "1", "-cluster_time_limit", "1000", "pipe:1",
	)
	hideCommandWindow(cmd)
	cmd.Stderr = log.Writer()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "FFmpeg output could not be opened")
		return
	}
	if err := cmd.Start(); err != nil {
		writeError(w, http.StatusInternalServerError, "FFmpeg could not start")
		return
	}
	closeJob, err := attachKillOnCloseJob(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		log.Printf("FFmpeg process guard failed: %v", err)
		writeError(w, http.StatusInternalServerError, "FFmpeg process guard could not start")
		return
	}
	defer closeJob()
	w.Header().Set("Content-Type", "video/webm")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	_, copyErr := io.Copy(w, stdout)
	waitErr := cmd.Wait()
	if copyErr != nil && streamContext.Err() == nil {
		log.Printf("FFmpeg stream copy: %v", copyErr)
	}
	if waitErr != nil && streamContext.Err() == nil {
		log.Printf("FFmpeg stopped: %v", waitErr)
	}
}

func transcodeVideoRates(height int) (bitrate, maxrate, bufferSize string) {
	switch height {
	case 240:
		return "350k", "500k", "800k"
	case 480:
		return "1100k", "1400k", "2200k"
	case 720:
		return "2200k", "3000k", "4500k"
	case 1080:
		return "4000k", "5200k", "8000k"
	default:
		return "650k", "850k", "1300k"
	}
}

func allowedHLSURL(value *url.URL) bool {
	if value == nil || value.Scheme != "https" || value.User != nil || value.Port() != "" {
		return false
	}
	host := strings.ToLower(value.Hostname())
	return host == "googlevideo.com" || strings.HasSuffix(host, ".googlevideo.com")
}

func rewriteHLSManifest(body string, base *url.URL, proxyBase string) string {
	rewrite := func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		resolved := base.ResolveReference(parsed)
		if !allowedHLSURL(resolved) {
			return raw
		}
		return proxyBase + url.QueryEscape(resolved.String())
	}
	lines := strings.Split(body, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			lines[index] = rewrite(trimmed)
			continue
		}
		lines[index] = hlsURIAttributePattern.ReplaceAllStringFunc(line, func(match string) string {
			parts := hlsURIAttributePattern.FindStringSubmatch(match)
			return `URI="` + rewrite(parts[1]) + `"`
		})
	}
	return strings.Join(lines, "\n")
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
	if previous.LaunchOnStartup != next.LaunchOnStartup {
		if err := setLaunchOnStartup(next.LaunchOnStartup); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := s.config.Save(next); err != nil {
		if previous.LaunchOnStartup != next.LaunchOnStartup {
			_ = setLaunchOnStartup(previous.LaunchOnStartup)
		}
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
	"heights":          func() []int { return []int{360, 480, 720, 1080} },
	"transcodeHeights": func() []int { return []int{240, 360, 480, 720, 1080} },
	"companionVersion": func() string { return version },
	"companionBuild":   func() string { return buildNumber },
	"protocolRange":    func() string { return fmt.Sprintf("%d-%d", protocolMinVersion, protocolMaxVersion) },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>ZZZ Wallpaper Companion</title><style>
:root{color-scheme:dark;font-family:Segoe UI,Arial,sans-serif;background:#111315;color:#f3f4f4}body{margin:0}.bar{height:5px;background:#f3c942}.wrap{max-width:620px;margin:0 auto;padding:36px 22px}h1{font-size:25px;margin:0 0 7px}.sub{color:#aeb5b8;margin:0 0 30px}.section{border-top:1px solid #34383a;padding:22px 0}label{display:grid;grid-template-columns:1fr 190px;gap:18px;align-items:center;margin:0 0 18px}small{display:block;color:#949b9e;margin-top:4px}.warning{color:#f3c942}input,select{box-sizing:border-box;width:100%;background:#1c2022;color:#fff;border:1px solid #4a5053;padding:9px;border-radius:4px;font:inherit}input[type=checkbox]{width:auto;justify-self:end}button{background:#f3c942;color:#171717;border:0;padding:10px 18px;border-radius:4px;font-weight:700;cursor:pointer}.status{margin-left:12px;color:#aeb5b8}@media(max-width:520px){label{grid-template-columns:1fr}}
</style></head><body><div class="bar"></div><main class="wrap"><h1>ZZZ Wallpaper Companion</h1><p class="sub">Version {{companionVersion}} · Build {{companionBuild}} · Protocol {{protocolRange}}</p><form id="settings"><div class="section">
<label><span>Companion port<small>Match this value in Wallpaper Engine. Restart required.</small></span><input id="port" type="number" min="1024" max="65535" value="{{.Port}}"></label>
<label><span>Maximum resolution<small>Higher resolutions use more bandwidth and GPU memory.</small></span><select id="height">{{range $v := heights}}<option>{{$v}}</option>{{end}}</select></label>
<label><span>Live-stream resolution<small class="warning">Higher resolutions substantially increase CPU usage while FFmpeg converts HLS to WebM. 1080p is experimental and may use heavy CPU.</small></span><select id="transcodeHeight">{{range $v := transcodeHeights}}<option>{{$v}}</option>{{end}}</select></label>
<label><span>yt-dlp channel<small>Nightly is recommended for timely YouTube fixes.</small></span><select id="channel"><option value="nightly">Nightly</option><option value="stable">Stable</option></select></label>
<label><span>Launch on Windows startup<small>Start quietly in the notification area after you sign in.</small></span><input id="startup" type="checkbox" {{if .LaunchOnStartup}}checked{{end}}></label>
<label><span>Automatic companion updates<small>Download compatible verified releases and install them on the next restart.</small></span><input id="autoUpdate" type="checkbox" {{if .AutoUpdate}}checked{{end}}></label>
</div><button type="submit">Save settings</button><span class="status" id="status"></span></form></main><script>
const initial={port:{{.Port}},maxHeight:{{.MaxHeight}},updateChannel:{{printf "%q" .UpdateChannel}},launchOnStartup:{{.LaunchOnStartup}},transcodeHeight:{{.TranscodeHeight}},autoUpdate:{{.AutoUpdate}}};
height.value=initial.maxHeight;channel.value=initial.updateChannel;transcodeHeight.value=initial.transcodeHeight;
settings.addEventListener('submit',async e=>{e.preventDefault();status.textContent='Saving...';try{const r=await fetch('/api/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({port:Number(port.value),maxHeight:Number(height.value),updateChannel:channel.value,launchOnStartup:startup.checked,transcodeHeight:Number(transcodeHeight.value),autoUpdate:autoUpdate.checked})});const j=await r.json();if(!r.ok)throw new Error(j.error);status.textContent=j.restartRequired?'Saved. Restart the companion.':'Saved.'}catch(e){status.textContent=e.message}});
</script></body></html>`))
