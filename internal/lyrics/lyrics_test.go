package lyrics

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestParseLRCMultipleTimestampsAndSort(t *testing.T) {
	lines := parseLRC("[00:20.50]second\n[00:10.00][00:12.25]first")
	if len(lines) != 3 || lines[0].TimeMS != 10000 || lines[1].TimeMS != 12250 || lines[2].TimeMS != 20500 {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestLyricsLookupHonorsCancellationDuringDebounce(t *testing.T) {
	service := NewService(t.TempDir(), "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err := service.Lookup(ctx, LyricsRequest{Artist: "Artist", Title: "Song"})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("lookup did not cancel promptly: %v", err)
	}
}

func TestLyricsCacheRoundTrip(t *testing.T) {
	service := NewService(t.TempDir(), "test")
	result := LyricsResponse{Status: "ready", TrackKey: "track", Source: "test", Synced: true, Lines: []LyricLine{{TimeMS: 1000, Text: "line"}}}
	service.store("track", result)
	service.memory = make(map[string]LyricsResponse)
	got, ok := service.cached("track")
	if !ok || len(got.Lines) != 1 || got.Lines[0].Text != "line" {
		t.Fatalf("cache mismatch: %#v", got)
	}
	if _, err := filepath.Abs(service.cacheDir); err != nil {
		t.Fatal(err)
	}
}

func TestLyricsCacheDoesNotRetainNegativeResults(t *testing.T) {
	service := NewService(t.TempDir(), "test")
	service.store("missing", LyricsResponse{Status: "not_found", TrackKey: "missing"})
	if _, ok := service.cached("missing"); ok {
		t.Fatal("not_found result was cached")
	}
}

func TestLyricsTrackKeyIsStable(t *testing.T) {
	a := lyricsTrackKey(LyricsRequest{Artist: " Artist ", Title: "Song", Duration: 180.1})
	b := lyricsTrackKey(LyricsRequest{Artist: "artist", Title: "song", Duration: 180.4})
	if a != b {
		t.Fatalf("equivalent metadata produced different keys: %s %s", a, b)
	}
}

func TestArtistSearchVariantsSplitsCollaborators(t *testing.T) {
	got := artistSearchVariants("Richie Jen, A Niu")
	want := []string{"Richie Jen, A Niu", "Richie Jen", "A Niu", ""}
	if len(got) != len(want) {
		t.Fatalf("unexpected variants: %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("variant %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMetadataMatchScoreTreatsChineseScriptsAsEquivalent(t *testing.T) {
	if score, ok := metadataMatchScore("再出發", "再出发"); !ok || score != 1 {
		t.Fatalf("Traditional/Simplified title did not match: score=%v ok=%v", score, ok)
	}
}

func TestSelectLRCLIBTrackMatchesOneCollaborator(t *testing.T) {
	track := LyricsRequest{Artist: "Richie Jen, A Niu", Title: "再出發", Duration: 285}
	candidates := []lrclibTrack{{
		TrackName: "再出發", ArtistName: "Richie Jen", Duration: 286, SyncedLyrics: "lyrics",
	}}
	got, ok := selectLRCLIBTrack(track, candidates)
	if !ok || got.SyncedLyrics != "lyrics" {
		t.Fatalf("collaboration candidate was not selected: %#v", got)
	}
}

func TestSelectNetEaseTrackMatchesSimplifiedChinese(t *testing.T) {
	candidate := neteaseTrack{ID: 1, Name: "再出发", Duration: 286000}
	track := LyricsRequest{Title: "再出發", Duration: 285}
	got, ok := selectNetEaseTrack(track, []neteaseTrack{candidate})
	if !ok || got.ID != candidate.ID {
		t.Fatalf("Simplified Chinese candidate was not selected: %#v", got)
	}
}

func TestNormalizeLyricsRequestPrefersOfficialVideoTitleArtist(t *testing.T) {
	got := normalizeLyricsRequest(LyricsRequest{
		Artist: "UNIVERSAL MUSIC JAPAN",
		Title:  "suis from ヨルシカ - 猫日 (OFFICIAL VIDEO)",
	})
	if got.Artist != "suis from ヨルシカ" || got.Title != "猫日" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
}

func TestNormalizeLyricsRequestExtractsJapaneseQuotedCollaborationTitle(t *testing.T) {
	got := normalizeLyricsRequest(LyricsRequest{
		Artist: "Creepy Nuts",
		Title:  "Creepy Nuts｢Bling-Bang-Bang-Born｣ × TV Anime｢マッシュル-MASHLE-｣ Collaboration Music Video #BBBBダンス",
	})
	if got.Artist != "Creepy Nuts" || got.Title != "Bling-Bang-Bang-Born" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
}

func TestSelectLRCLIBTrackToleratesVideoDurationAndAlbum(t *testing.T) {
	track := LyricsRequest{Artist: "Owl City", Title: "Fireflies", Album: "YouTube", Duration: 232}
	candidates := []lrclibTrack{
		{TrackName: "Fireflies (Live)", ArtistName: "Owl City", Duration: 232, SyncedLyrics: "live"},
		{TrackName: "Fireflies", ArtistName: "Owl City", AlbumName: "Ocean Eyes", Duration: 242, SyncedLyrics: "album"},
		{TrackName: "Owl City - Fireflies (Official Music Video)", ArtistName: "Owl City", Duration: 232, SyncedLyrics: "video"},
	}
	got, ok := selectLRCLIBTrack(track, candidates)
	if !ok || got.SyncedLyrics != "video" {
		t.Fatalf("selected %#v", got)
	}
}

func TestSelectLRCLIBTrackRejectsWrongArtistAndVersion(t *testing.T) {
	track := LyricsRequest{Artist: "Example Artist", Title: "Example Song (Acoustic)", Duration: 200}
	candidates := []lrclibTrack{
		{TrackName: "Example Song", ArtistName: "Example Artist", Duration: 200, SyncedLyrics: "studio"},
		{TrackName: "Example Song (Acoustic)", ArtistName: "Other Artist", Duration: 200, SyncedLyrics: "wrong artist"},
		{TrackName: "Example Song (Acoustic)", ArtistName: "Example Artist", Duration: 205, SyncedLyrics: "correct"},
	}
	got, ok := selectLRCLIBTrack(track, candidates)
	if !ok || got.SyncedLyrics != "correct" {
		t.Fatalf("selected %#v", got)
	}
}

func TestSelectLRCLIBTrackRejectsUnrequestedVersion(t *testing.T) {
	track := LyricsRequest{Artist: "Example Artist", Title: "Example Song", Duration: 200}
	candidates := []lrclibTrack{
		{TrackName: "Example Song (Live)", ArtistName: "Example Artist", Duration: 200, SyncedLyrics: "wrong version"},
		{TrackName: "Example Song Remix", ArtistName: "Example Artist", Duration: 200, SyncedLyrics: "wrong remix"},
	}
	if got, ok := selectLRCLIBTrack(track, candidates); ok {
		t.Fatalf("selected unrequested version: %#v", got)
	}
}

func TestSelectLRCLIBTrackRejectsLooseSubstring(t *testing.T) {
	track := LyricsRequest{Artist: "Example Artist", Title: "Home", Duration: 200}
	candidates := []lrclibTrack{
		{TrackName: "Take Me Home Tonight", ArtistName: "Example Artist", Duration: 200, SyncedLyrics: "wrong song"},
	}
	if got, ok := selectLRCLIBTrack(track, candidates); ok {
		t.Fatalf("selected loose substring: %#v", got)
	}
}

func TestSelectLRCLIBTrackRejectsLargeDurationMismatch(t *testing.T) {
	track := LyricsRequest{Artist: "Example Artist", Title: "Example Song", Duration: 200}
	candidates := []lrclibTrack{
		{TrackName: "Example Song", ArtistName: "Example Artist", Duration: 245, SyncedLyrics: "wrong recording"},
	}
	if got, ok := selectLRCLIBTrack(track, candidates); ok {
		t.Fatalf("selected mismatched duration: %#v", got)
	}
}

func TestSelectNetEaseTrackRanksTitleVersionAndDuration(t *testing.T) {
	makeTrack := func(id int64, name, artist string, duration int64) neteaseTrack {
		candidate := neteaseTrack{ID: id, Name: name, Duration: duration}
		candidate.Artists = append(candidate.Artists, struct {
			Name string `json:"name"`
		}{Name: artist})
		return candidate
	}
	track := LyricsRequest{Artist: "Example Artist", Title: "Example Song (Acoustic)", Duration: 205}
	candidates := []neteaseTrack{
		makeTrack(1, "Example Song", "Example Artist", 205000),
		makeTrack(2, "Example Song (Acoustic)", "Other Artist", 205000),
		makeTrack(3, "Example Song (Acoustic)", "Example Artist", 207000),
	}
	got, ok := selectNetEaseTrack(track, candidates)
	if !ok || got.ID != 3 {
		t.Fatalf("selected %#v", got)
	}
}

func TestSelectNetEaseTrackAcceptsArtistFromGroupCredit(t *testing.T) {
	candidate := neteaseTrack{ID: 3407565594, Name: "猫日", Duration: 230000}
	candidate.Artists = append(candidate.Artists, struct {
		Name string `json:"name"`
	}{Name: "suis"})

	track := LyricsRequest{Artist: "suis from ヨルシカ", Title: "猫日", Duration: 230}
	got, ok := selectNetEaseTrack(track, []neteaseTrack{candidate})
	if !ok || got.ID != candidate.ID {
		t.Fatalf("artist-from-group credit was not matched: %#v", got)
	}
}
