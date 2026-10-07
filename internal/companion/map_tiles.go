package companion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Only visible map tiles are requested by the wallpaper. This native client
// identifies the application without inventing a website Referer for file://.
type mapTileService struct {
	dir          string
	baseURL      string
	client       *http.Client
	locks        [32]sync.Mutex
	slots        chan struct{}
	mu           sync.Mutex
	blockedUntil time.Time
}

type cachedMapTile struct {
	Body     []byte        `json:"body"`
	Expires  time.Time     `json:"expires"`
	TTL      time.Duration `json:"ttl"`
	ETag     string        `json:"etag,omitempty"`
	Modified string        `json:"modified,omitempty"`
}

func newMapTileService(dataDir string) *mapTileService {
	return &mapTileService{
		dir:     filepath.Join(dataDir, "map-tiles"),
		baseURL: "https://tile.openstreetmap.org",
		client:  &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		slots:   make(chan struct{}, 4),
	}
}

func mapTileTTL(h http.Header, now time.Time, fallback time.Duration) (time.Duration, bool) {
	var ttl = fallback
	noCache := false
	maxAge := false
	for _, item := range strings.Split(h.Get("Cache-Control"), ",") {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "no-store" {
			return 0, false
		}
		if item == "no-cache" {
			noCache = true
		}
		if strings.HasPrefix(item, "max-age=") {
			seconds, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(item, "max-age="), "\""), 10, 32)
			if err == nil && seconds >= 0 {
				age, _ := strconv.ParseInt(h.Get("Age"), 10, 32)
				ttl = time.Duration(max(0, seconds-max(0, age))) * time.Second
				maxAge = true
			}
		}
	}
	if noCache {
		return 0, true
	}
	if !maxAge {
		if expires, err := http.ParseTime(h.Get("Expires")); err == nil {
			ttl = max(0, expires.Sub(now))
		}
	}
	return ttl, true
}

func (s *mapTileService) serve(w http.ResponseWriter, r *http.Request) {
	z, errZ := strconv.Atoi(r.PathValue("z"))
	x, errX := strconv.Atoi(r.PathValue("x"))
	y, errY := strconv.Atoi(r.PathValue("y"))
	if errZ != nil || errX != nil || errY != nil || z < 0 || z > 19 || x < 0 || y < 0 || x >= 1<<z || y >= 1<<z {
		writeError(w, http.StatusBadRequest, "invalid map tile coordinates")
		return
	}
	lock := &s.locks[(x+y+z)%len(s.locks)]
	lock.Lock()
	defer lock.Unlock()
	if r.Context().Err() != nil {
		return
	}
	path := filepath.Join(s.dir, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".json")
	var cached cachedMapTile
	if body, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(body, &cached)
	}
	if !bytes.HasPrefix(cached.Body, []byte("\x89PNG\r\n\x1a\n")) {
		cached = cachedMapTile{}
	}
	if len(cached.Body) > 0 && time.Now().Before(cached.Expires) {
		sendMapTile(w, cached)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-r.Context().Done():
		return
	}
	s.mu.Lock()
	blocked := time.Now().Before(s.blockedUntil)
	s.mu.Unlock()
	if blocked {
		writeError(w, http.StatusServiceUnavailable, "map provider unavailable; requests paused")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, fmt.Sprintf("%s/%d/%d/%d.png", s.baseURL, z, x, y), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cannot request map tile")
		return
	}
	request.Header.Set("User-Agent", "ZZZWallpaperCompanion/"+version+" (+https://github.com/Garruvex/zzz-wallpaper-companion)")
	if len(cached.Body) > 0 {
		if cached.ETag != "" {
			request.Header.Set("If-None-Match", cached.ETag)
		}
		if cached.Modified != "" {
			request.Header.Set("If-Modified-Since", cached.Modified)
		}
	}
	response, err := s.client.Do(request)
	if err != nil {
		writeError(w, http.StatusBadGateway, "map provider unreachable")
		return
	}
	defer response.Body.Close()
	now := time.Now()
	if response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		until := now.Add(time.Hour)
		if seconds, err := strconv.ParseInt(response.Header.Get("Retry-After"), 10, 32); err == nil && seconds > 0 {
			until = maxTime(until, now.Add(time.Duration(seconds)*time.Second))
		} else if date, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
			until = maxTime(until, date)
		}
		s.mu.Lock()
		s.blockedUntil = maxTime(s.blockedUntil, until)
		s.mu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "map provider blocked access; requests paused")
		return
	}
	if response.StatusCode == http.StatusNotModified && len(cached.Body) > 0 {
		// A 304 may omit cache headers; retain the previous cache lifetime.
	} else if response.StatusCode == http.StatusOK {
		body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 || !bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) {
			writeError(w, http.StatusBadGateway, "invalid map tile response")
			return
		}
		cached = cachedMapTile{Body: body, TTL: 7 * 24 * time.Hour}
	} else {
		writeError(w, http.StatusBadGateway, "map provider returned an error")
		return
	}
	if value := response.Header.Get("ETag"); value != "" {
		cached.ETag = value
	}
	if value := response.Header.Get("Last-Modified"); value != "" {
		cached.Modified = value
	}
	ttl, persist := mapTileTTL(response.Header, now, cached.TTL)
	cached.TTL = ttl
	cached.Expires = now.Add(ttl)
	if persist {
		// Fail closed if persistent caching is unavailable: don't repeatedly
		// download a tile on each wallpaper restart.
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			writeError(w, 500, "map cache unavailable")
			return
		}
		body, err := json.Marshal(cached)
		if err == nil {
			err = os.WriteFile(path, body, 0o600)
		}
		if err != nil {
			writeError(w, 500, "cannot save map cache")
			return
		}
	} else {
		_ = os.Remove(path)
	}
	sendMapTile(w, cached)
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

func sendMapTile(w http.ResponseWriter, tile cachedMapTile) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", max(0, int(time.Until(tile.Expires).Seconds()))))
	_, _ = w.Write(tile.Body)
}
