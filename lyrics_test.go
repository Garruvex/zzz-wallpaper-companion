package main

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
	service := newLyricsService(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_, err := service.Lookup(ctx, LyricsRequest{Artist: "Artist", Title: "Song"})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("lookup did not cancel promptly: %v", err)
	}
}

func TestLyricsCacheRoundTrip(t *testing.T) {
	service := newLyricsService(t.TempDir())
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

func TestLyricsTrackKeyIsStable(t *testing.T) {
	a := lyricsTrackKey(LyricsRequest{Artist: " Artist ", Title: "Song", Duration: 180.1})
	b := lyricsTrackKey(LyricsRequest{Artist: "artist", Title: "song", Duration: 180.4})
	if a != b {
		t.Fatalf("equivalent metadata produced different keys: %s %s", a, b)
	}
}
