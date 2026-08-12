package lyrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxLyricsResponseBytes = 1 << 20

type LyricsRequest struct {
	Artist   string  `json:"artist"`
	Title    string  `json:"title"`
	Album    string  `json:"album,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

type LyricLine struct {
	TimeMS int64  `json:"timeMs"`
	Text   string `json:"text"`
}

type LyricsResponse struct {
	Status       string      `json:"status"`
	TrackKey     string      `json:"trackKey"`
	Source       string      `json:"source,omitempty"`
	Synced       bool        `json:"synced"`
	Instrumental bool        `json:"instrumental,omitempty"`
	Lines        []LyricLine `json:"lines,omitempty"`
	PlainLines   []string    `json:"plainLines,omitempty"`
}

type LyricsService struct {
	cacheDir      string
	clientVersion string
	client        *http.Client
	mu            sync.RWMutex
	memory        map[string]LyricsResponse
	onlineMu      sync.Mutex
	requestMu     sync.Mutex
	currentKey    string
	cancelCurrent context.CancelFunc
	requestID     uint64
}

func NewService(dataDir, clientVersion string) *LyricsService {
	return &LyricsService{
		cacheDir:      filepath.Join(dataDir, "lyrics"),
		clientVersion: clientVersion,
		client:        &http.Client{Timeout: 8 * time.Second},
		memory:        make(map[string]LyricsResponse),
	}
}

func lyricsTrackKey(r LyricsRequest) string {
	normalized := strings.ToLower(strings.Join([]string{strings.TrimSpace(r.Artist), strings.TrimSpace(r.Title), strings.TrimSpace(r.Album), strconv.Itoa(int(r.Duration + .5))}, "|"))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:16])
}

func (s *LyricsService) Lookup(ctx context.Context, req LyricsRequest) (LyricsResponse, error) {
	req.Artist, req.Title, req.Album = strings.TrimSpace(req.Artist), strings.TrimSpace(req.Title), strings.TrimSpace(req.Album)
	req = normalizeLyricsRequest(req)
	if req.Title == "" || len(req.Title) > 500 || len(req.Artist) > 500 || len(req.Album) > 500 || req.Duration < 0 {
		return LyricsResponse{}, errors.New("invalid track metadata")
	}
	key := lyricsTrackKey(req)
	if cached, ok := s.cached(key); ok {
		return cached, nil
	}
	lookupCtx, cancel := context.WithCancel(ctx)
	s.requestMu.Lock()
	if s.cancelCurrent != nil {
		s.cancelCurrent()
	}
	s.requestID++
	requestID := s.requestID
	s.currentKey, s.cancelCurrent = key, cancel
	s.requestMu.Unlock()
	defer func() {
		cancel()
		s.requestMu.Lock()
		if s.requestID == requestID {
			s.currentKey, s.cancelCurrent = "", nil
		}
		s.requestMu.Unlock()
	}()

	select {
	case <-time.After(2 * time.Second):
	case <-lookupCtx.Done():
		return LyricsResponse{}, lookupCtx.Err()
	}
	if cached, ok := s.cached(key); ok {
		return cached, nil
	}

	s.onlineMu.Lock()
	defer s.onlineMu.Unlock()
	if cached, ok := s.cached(key); ok {
		return cached, nil
	}

	result, err := s.lookupLRCLIB(lookupCtx, req, key)
	if err != nil || (result.Status != "ready" && result.Status != "instrumental") {
		result, err = s.lookupNetEase(lookupCtx, req, key)
	}
	if err != nil {
		return LyricsResponse{Status: "error", TrackKey: key}, err
	}
	if result.Status == "" {
		result = LyricsResponse{Status: "not_found", TrackKey: key}
	}
	s.store(key, result)
	return result, nil
}

func (s *LyricsService) cached(key string) (LyricsResponse, bool) {
	s.mu.RLock()
	value, ok := s.memory[key]
	s.mu.RUnlock()
	if ok {
		return value, true
	}
	body, err := os.ReadFile(filepath.Join(s.cacheDir, key+".json"))
	if err != nil || len(body) > maxLyricsResponseBytes {
		return LyricsResponse{}, false
	}
	var result LyricsResponse
	if json.Unmarshal(body, &result) != nil || result.TrackKey != key {
		return LyricsResponse{}, false
	}
	s.mu.Lock()
	s.memory[key] = result
	s.mu.Unlock()
	return result, true
}

func (s *LyricsService) store(key string, result LyricsResponse) {
	if result.Status != "ready" && result.Status != "instrumental" {
		return
	}
	s.mu.Lock()
	s.memory[key] = result
	s.mu.Unlock()
	body, err := json.Marshal(result)
	if err != nil || len(body) > maxLyricsResponseBytes {
		return
	}
	if os.MkdirAll(s.cacheDir, 0o700) == nil {
		_ = os.WriteFile(filepath.Join(s.cacheDir, key+".json"), body, 0o600)
	}
}

func (s *LyricsService) lookupLRCLIB(ctx context.Context, track LyricsRequest, key string) (LyricsResponse, error) {
	endpoint, _ := url.Parse("https://lrclib.net/api/get")
	q := endpoint.Query()
	q.Set("track_name", track.Title)
	q.Set("artist_name", track.Artist)
	if track.Duration > 0 {
		q.Set("duration", strconv.Itoa(int(track.Duration+.5)))
	}
	endpoint.RawQuery = q.Encode()
	var body struct {
		Synced, Plain string
		Instrumental  bool
	}
	var raw struct {
		SyncedLyrics string `json:"syncedLyrics"`
		PlainLyrics  string `json:"plainLyrics"`
		Instrumental bool   `json:"instrumental"`
	}
	status, err := s.getJSON(ctx, endpoint.String(), "zzz-wallpaper-companion/"+s.clientVersion, "", &raw)
	if err != nil {
		return LyricsResponse{}, err
	}
	if status == http.StatusNotFound {
		return s.searchLRCLIB(ctx, track, key)
	}
	body.Synced, body.Plain, body.Instrumental = raw.SyncedLyrics, raw.PlainLyrics, raw.Instrumental
	return makeLyricsResponse(key, "lrclib", body.Synced, body.Plain, body.Instrumental), nil
}

type lrclibTrack struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	SyncedLyrics string  `json:"syncedLyrics"`
	PlainLyrics  string  `json:"plainLyrics"`
	Instrumental bool    `json:"instrumental"`
}

func (s *LyricsService) searchLRCLIB(ctx context.Context, track LyricsRequest, key string) (LyricsResponse, error) {
	endpoint, _ := url.Parse("https://lrclib.net/api/search")
	q := endpoint.Query()
	q.Set("track_name", track.Title)
	if track.Artist != "" {
		q.Set("artist_name", track.Artist)
	}
	endpoint.RawQuery = q.Encode()
	var candidates []lrclibTrack
	status, err := s.getJSON(ctx, endpoint.String(), "zzz-wallpaper-companion/"+s.clientVersion, "", &candidates)
	if err != nil {
		return LyricsResponse{}, err
	}
	if status == http.StatusNotFound || len(candidates) == 0 {
		return LyricsResponse{Status: "not_found", TrackKey: key}, nil
	}
	match, ok := selectLRCLIBTrack(track, candidates)
	if !ok {
		return LyricsResponse{Status: "not_found", TrackKey: key}, nil
	}
	return makeLyricsResponse(key, "lrclib", match.SyncedLyrics, match.PlainLyrics, match.Instrumental), nil
}

var metadataSuffix = regexp.MustCompile(`(?i)\s*[\[(](?:official\s+)?(?:(?:music\s+)?video(?:\s+clip)?|audio|lyrics?|lyric\s+video|visuali[sz]er)[\])]`)
var artistTitleSeparator = regexp.MustCompile(`\s+[-–—]\s+`)
var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)
var versionMarker = regexp.MustCompile(`(?i)\b(live|remix|acoustic|instrumental|karaoke|demo|edit|version|cover|sped\s*up|slowed)\b`)

func normalizedMetadata(value string) string {
	value = metadataSuffix.ReplaceAllString(strings.ToLower(value), " ")
	return strings.TrimSpace(nonWord.ReplaceAllString(value, " "))
}

func normalizeLyricsRequest(track LyricsRequest) LyricsRequest {
	originalTitle := strings.TrimSpace(track.Title)
	cleanTitle := strings.TrimSpace(metadataSuffix.ReplaceAllString(originalTitle, " "))
	if cleanTitle != originalTitle {
		if separator := artistTitleSeparator.FindStringIndex(cleanTitle); separator != nil {
			artist := strings.TrimSpace(cleanTitle[:separator[0]])
			title := strings.TrimSpace(cleanTitle[separator[1]:])
			if artist != "" && title != "" {
				track.Artist, track.Title = artist, title
				return track
			}
		}
		track.Title = cleanTitle
	}
	return track
}

func metadataMatchScore(want, got string) (float64, bool) {
	want, got = normalizedMetadata(want), normalizedMetadata(got)
	if want == "" {
		return 0, true
	}
	if got == "" {
		return 0, false
	}
	if want == got {
		return 1, true
	}
	if strings.Contains(want, got) || strings.Contains(got, want) {
		shorter, longer := len(want), len(got)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		return .7 + .2*float64(shorter)/float64(longer), true
	}
	return 0, false
}

func candidateTitle(candidate lrclibTrack) string {
	title := normalizedMetadata(candidate.TrackName)
	artist := normalizedMetadata(candidate.ArtistName)
	if artist != "" && strings.HasPrefix(title, artist+" ") {
		return strings.TrimSpace(strings.TrimPrefix(title, artist))
	}
	return title
}

func selectLRCLIBTrack(track LyricsRequest, candidates []lrclibTrack) (lrclibTrack, bool) {
	bestScore := -1.0
	var best lrclibTrack
	wantVersion := versionMarker.FindString(normalizedMetadata(track.Title))
	for _, candidate := range candidates {
		titleScore, titleMatch := metadataMatchScore(track.Title, candidateTitle(candidate))
		artistScore, artistMatch := metadataMatchScore(track.Artist, candidate.ArtistName)
		if !titleMatch || !artistMatch {
			continue
		}
		score := titleScore*60 + artistScore*40
		gotVersion := versionMarker.FindString(normalizedMetadata(candidate.TrackName))
		if wantVersion == gotVersion {
			score += 12
		} else if wantVersion != "" || gotVersion != "" {
			score -= 25
		}
		if track.Duration > 0 && candidate.Duration > 0 {
			difference := math.Abs(track.Duration - candidate.Duration)
			score += math.Max(0, 20-difference/1.5)
		}
		if albumScore, ok := metadataMatchScore(track.Album, candidate.AlbumName); ok {
			score += albumScore * 5
		}
		if strings.TrimSpace(candidate.SyncedLyrics) != "" {
			score += 8
		} else if strings.TrimSpace(candidate.PlainLyrics) != "" {
			score += 2
		}
		if score > bestScore {
			bestScore, best = score, candidate
		}
	}
	return best, bestScore >= 0
}

func (s *LyricsService) lookupNetEase(ctx context.Context, track LyricsRequest, key string) (LyricsResponse, error) {
	endpoint, _ := url.Parse("https://music.163.com/api/search/get")
	q := endpoint.Query()
	q.Set("s", strings.TrimSpace(track.Title+" "+track.Artist))
	q.Set("type", "1")
	q.Set("limit", "10")
	endpoint.RawQuery = q.Encode()
	var search struct {
		Result struct {
			Songs []neteaseTrack `json:"songs"`
		} `json:"result"`
	}
	_, err := s.getJSON(ctx, endpoint.String(), "Mozilla/5.0", "https://music.163.com/", &search)
	if err != nil {
		return LyricsResponse{}, err
	}
	match, ok := selectNetEaseTrack(track, search.Result.Songs)
	if !ok {
		return LyricsResponse{Status: "not_found", TrackKey: key}, nil
	}
	lyricsURL := fmt.Sprintf("https://music.163.com/api/song/lyric?id=%d&lv=1&kv=1&tv=-1", match.ID)
	var lyric struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	_, err = s.getJSON(ctx, lyricsURL, "Mozilla/5.0", "https://music.163.com/", &lyric)
	if err != nil {
		return LyricsResponse{}, err
	}
	return makeLyricsResponse(key, "netease", lyric.LRC.Lyric, "", false), nil
}

type neteaseTrack struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Duration int64  `json:"duration"`
	Artists  []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name string `json:"name"`
	} `json:"album"`
}

func selectNetEaseTrack(track LyricsRequest, candidates []neteaseTrack) (neteaseTrack, bool) {
	bestScore := -1.0
	var best neteaseTrack
	wantVersion := versionMarker.FindString(normalizedMetadata(track.Title))
	for _, candidate := range candidates {
		artistNames := make([]string, 0, len(candidate.Artists))
		for _, artist := range candidate.Artists {
			artistNames = append(artistNames, artist.Name)
		}
		titleScore, titleMatch := metadataMatchScore(track.Title, candidate.Name)
		artistScore, artistMatch := metadataMatchScore(track.Artist, strings.Join(artistNames, " "))
		if !titleMatch || !artistMatch {
			continue
		}
		score := titleScore*60 + artistScore*40
		gotVersion := versionMarker.FindString(normalizedMetadata(candidate.Name))
		if wantVersion == gotVersion {
			score += 12
		} else if wantVersion != "" || gotVersion != "" {
			score -= 25
		}
		if track.Duration > 0 && candidate.Duration > 0 {
			difference := math.Abs(track.Duration - float64(candidate.Duration)/1000)
			score += math.Max(0, 20-difference/1.5)
		}
		if albumScore, ok := metadataMatchScore(track.Album, candidate.Album.Name); ok {
			score += albumScore * 5
		}
		if score > bestScore {
			bestScore, best = score, candidate
		}
	}
	return best, bestScore >= 0
}

func (s *LyricsService) getJSON(ctx context.Context, endpoint, userAgent, referer string, target any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("lyrics provider returned HTTP %d", resp.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxLyricsResponseBytes))
	err = decoder.Decode(target)
	return resp.StatusCode, err
}

var lrcTimestamp = regexp.MustCompile(`\[(\d+):(\d+(?:\.\d+)?)\]`)

func makeLyricsResponse(key, source, synced, plain string, instrumental bool) LyricsResponse {
	if instrumental {
		return LyricsResponse{Status: "instrumental", TrackKey: key, Source: source, Instrumental: true}
	}
	lines := parseLRC(synced)
	if len(lines) > 0 {
		return LyricsResponse{Status: "ready", TrackKey: key, Source: source, Synced: true, Lines: lines}
	}
	plainLines := cleanPlainLines(plain)
	if len(plainLines) > 0 {
		return LyricsResponse{Status: "ready", TrackKey: key, Source: source, PlainLines: plainLines}
	}
	return LyricsResponse{Status: "not_found", TrackKey: key}
}

func parseLRC(text string) []LyricLine {
	result := make([]LyricLine, 0)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		matches := lrcTimestamp.FindAllStringSubmatch(raw, -1)
		if len(matches) == 0 {
			continue
		}
		lyric := strings.TrimSpace(lrcTimestamp.ReplaceAllString(raw, ""))
		if len(lyric) > 2000 {
			lyric = lyric[:2000]
		}
		for _, match := range matches {
			mins, _ := strconv.Atoi(match[1])
			secs, _ := strconv.ParseFloat(match[2], 64)
			ms := int64(float64(mins)*60000 + secs*1000)
			if ms >= 0 {
				result = append(result, LyricLine{TimeMS: ms, Text: lyric})
			}
			if len(result) >= 10000 {
				break
			}
		}
		if len(result) >= 10000 {
			break
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].TimeMS < result[j].TimeMS })
	return result
}

func cleanPlainLines(text string) []string {
	result := make([]string, 0)
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > 2000 {
			line = line[:2000]
		}
		result = append(result, line)
		if len(result) >= 10000 {
			break
		}
	}
	return result
}

func containsFold(a, b string) bool {
	return b != "" && strings.Contains(strings.ToLower(a), strings.ToLower(b))
}
