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
