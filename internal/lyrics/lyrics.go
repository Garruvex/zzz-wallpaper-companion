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

	"github.com/caiguanhao/opencc/configs/t2s"
)

const maxLyricsResponseBytes = 1 << 20

// Bump whenever normalization or candidate selection changes so successful
// results stored under an older interpretation cannot mask the new pipeline.
const lyricsMatcherVersion = "v4"

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
	normalized := strings.ToLower(strings.Join([]string{lyricsMatcherVersion, strings.TrimSpace(r.Artist), strings.TrimSpace(r.Title), strings.TrimSpace(r.Album), strconv.Itoa(int(r.Duration + .5))}, "|"))
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
	var candidates []lrclibTrack
	seen := make(map[string]bool)
	for _, artist := range artistSearchVariants(track.Artist) {
		endpoint, _ := url.Parse("https://lrclib.net/api/search")
		q := endpoint.Query()
		q.Set("track_name", track.Title)
		if artist != "" {
			q.Set("artist_name", artist)
		}
		endpoint.RawQuery = q.Encode()
		var found []lrclibTrack
		_, err := s.getJSON(ctx, endpoint.String(), "zzz-wallpaper-companion/"+s.clientVersion, "", &found)
		if err != nil {
			return LyricsResponse{}, err
		}
		for _, candidate := range found {
			id := normalizedMetadata(candidate.TrackName) + "|" + normalizedMetadata(candidate.ArtistName) + "|" + strconv.Itoa(int(candidate.Duration+.5))
			if !seen[id] {
				seen[id], candidates = true, append(candidates, candidate)
			}
		}
		if match, ok := selectLRCLIBTrack(track, candidates); ok {
			return makeLyricsResponse(key, "lrclib", match.SyncedLyrics, match.PlainLyrics, match.Instrumental), nil
		}
	}
	if len(candidates) == 0 {
		return LyricsResponse{Status: "not_found", TrackKey: key}, nil
	}
	return LyricsResponse{Status: "not_found", TrackKey: key}, nil
}

var metadataSuffix = regexp.MustCompile(`(?i)\s*[\[(](?:official\s+)?(?:(?:music\s+)?video(?:\s+clip)?|audio|lyrics?|lyric\s+video|visuali[sz]er)[\])]`)
var japaneseQuotedTitle = regexp.MustCompile(`[｢「]([^｣」]+)[｣」]`)
var artistTitleSeparator = regexp.MustCompile(`\s+[-–—]\s+`)
var nonWord = regexp.MustCompile(`[^\p{L}\p{N}]+`)
var versionMarker = regexp.MustCompile(`(?i)\b(live|remix|acoustic|instrumental|karaoke|demo|edit|version|cover|sped\s*up|slowed)\b`)
var artistSeparator = regexp.MustCompile(`(?i)\s*(?:,|&|/|;|、|，|\bfeat(?:uring)?\.?\b|\bft\.?\b|和|與|与)\s*`)

func artistSearchVariants(artist string) []string {
	artist = strings.TrimSpace(artist)
	variants := []string{artist}
	seen := map[string]bool{strings.ToLower(artist): true}
	for _, part := range artistSeparator.Split(artist, -1) {
		part = strings.TrimSpace(part)
		key := strings.ToLower(part)
		if part != "" && !seen[key] {
			seen[key], variants = true, append(variants, part)
		}
	}
	if !seen[""] {
		variants = append(variants, "")
	}
	return variants
}

func normalizedMetadata(value string) string {
	value = metadataSuffix.ReplaceAllString(strings.ToLower(value), " ")
	return strings.TrimSpace(nonWord.ReplaceAllString(value, " "))
}

func simplifyChinese(value string) string {
	return t2s.Dicts.Convert(value)
}

func normalizeLyricsRequest(track LyricsRequest) LyricsRequest {
	originalTitle := strings.TrimSpace(track.Title)
	// Japanese music channels commonly publish collaboration headlines as
	// Artist｢Track｣ × TV Anime ... . When the prefix agrees with the supplied
	// artist, the quoted portion is the reliable music title.
	if match := japaneseQuotedTitle.FindStringSubmatch(originalTitle); len(match) == 2 {
		prefix := strings.TrimSpace(originalTitle[:strings.Index(originalTitle, match[0])])
		if score, ok := metadataMatchScore(track.Artist, prefix); ok && score >= .8 {
			track.Title = strings.TrimSpace(match[1])
			return track
		}
	}
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
	// Providers in mainland China commonly catalogue Traditional metadata in
	// Simplified Chinese. Compare both sides in the same script as well.
	if simplifyChinese(want) == simplifyChinese(got) {
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

func artistMatchScore(want, got string) (float64, bool) {
	best := 0.0
	for _, artist := range artistSearchVariants(want) {
		if artist == "" {
			continue
		}
		if score, ok := metadataMatchScore(artist, got); ok && score > best {
			best = score
		}
	}
	if best >= .8 {
		return best, true
	}
	score, ok := metadataMatchScore(want, got)
	if !ok {
		return 0, false
	}
	if score >= .8 {
		return score, true
	}

	// Music metadata often brands a vocalist as "name from group", while
	// providers catalogue the same recording under the vocalist alone.
	// Keep this exception deliberately narrow: the shorter name must be a
	// complete prefix and the longer value must use an explicit "from" credit.
	wantNormalized := normalizedMetadata(want)
	gotNormalized := normalizedMetadata(got)
	shorter, longer := wantNormalized, gotNormalized
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len([]rune(shorter)) >= 3 && strings.HasPrefix(longer, shorter+" from ") {
		return .9, true
	}
	return score, true
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
		artistScore, artistMatch := artistMatchScore(track.Artist, candidate.ArtistName)
		if !titleMatch || titleScore < .85 || !artistMatch || (strings.TrimSpace(track.Artist) != "" && artistScore < .8) {
			continue
		}
		score := titleScore*60 + artistScore*40
		gotVersion := versionMarker.FindString(normalizedMetadata(candidate.TrackName))
		if wantVersion == gotVersion {
			score += 12
		} else {
			continue
		}
		if track.Duration > 0 && candidate.Duration > 0 {
			difference := math.Abs(track.Duration - candidate.Duration)
			if difference > 20 {
				continue
			}
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
	return best, bestScore >= 85
}

func (s *LyricsService) lookupNetEase(ctx context.Context, track LyricsRequest, key string) (LyricsResponse, error) {
	var candidates []neteaseTrack
	var match neteaseTrack
	matched := false
	seen := make(map[int64]bool)
	titles := []string{track.Title}
	if simplified := simplifyChinese(track.Title); simplified != track.Title {
		titles = append(titles, simplified)
	}
searches:
	for _, title := range titles {
		for _, artist := range artistSearchVariants(track.Artist) {
			endpoint, _ := url.Parse("https://music.163.com/api/search/get")
			q := endpoint.Query()
			q.Set("s", strings.TrimSpace(title+" "+simplifyChinese(artist)))
			q.Set("type", "1")
			q.Set("limit", "10")
			endpoint.RawQuery = q.Encode()
			var search struct {
				Result struct {
					Songs []neteaseTrack `json:"songs"`
				} `json:"result"`
			}
			if _, err := s.getJSON(ctx, endpoint.String(), "Mozilla/5.0", "https://music.163.com/", &search); err != nil {
				return LyricsResponse{}, err
			}
			for _, candidate := range search.Result.Songs {
				if !seen[candidate.ID] {
					seen[candidate.ID], candidates = true, append(candidates, candidate)
				}
			}
			if match, matched = selectNetEaseTrack(track, candidates); matched {
				break searches
			}
			if artist == "" && track.Duration > 0 {
				fallbackTrack := track
				fallbackTrack.Artist = ""
				if match, matched = selectNetEaseTrack(fallbackTrack, candidates); matched {
					break searches
				}
			}
		}
	}
	if !matched {
		return LyricsResponse{Status: "not_found", TrackKey: key}, nil
	}
	lyricsURL := fmt.Sprintf("https://music.163.com/api/song/lyric?id=%d&lv=1&kv=1&tv=-1", match.ID)
	var lyric struct {
		LRC struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	_, err := s.getJSON(ctx, lyricsURL, "Mozilla/5.0", "https://music.163.com/", &lyric)
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
		artistScore, artistMatch := artistMatchScore(track.Artist, strings.Join(artistNames, " "))
		if !titleMatch || titleScore < .85 || !artistMatch || (strings.TrimSpace(track.Artist) != "" && artistScore < .8) {
			continue
		}
		score := titleScore*60 + artistScore*40
		gotVersion := versionMarker.FindString(normalizedMetadata(candidate.Name))
		if wantVersion == gotVersion {
			score += 12
		} else {
			continue
		}
		if track.Duration > 0 && candidate.Duration > 0 {
			difference := math.Abs(track.Duration - float64(candidate.Duration)/1000)
			if difference > 20 {
				continue
			}
			score += math.Max(0, 20-difference/1.5)
		}
		if albumScore, ok := metadataMatchScore(track.Album, candidate.Album.Name); ok {
			score += albumScore * 5
		}
		if score > bestScore {
			bestScore, best = score, candidate
		}
	}
	return best, bestScore >= 85
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
