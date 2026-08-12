package companion

import "testing"

func TestResolvedMusicMetadataPrefersMusicFields(t *testing.T) {
	track, artist := resolvedMusicMetadata(ytOutput{
		Title: "Artist - Song (Official Video)", Track: "Song", Artist: "Artist",
		Creator: "Creator", Uploader: "Label",
	})
	if track != "Song" || artist != "Artist" {
		t.Fatalf("unexpected music metadata: track=%q artist=%q", track, artist)
	}
}

func TestResolvedMusicMetadataFallsBackToDisplayMetadata(t *testing.T) {
	track, artist := resolvedMusicMetadata(ytOutput{Title: "Song", Creator: "Artist", Uploader: "Label"})
	if track != "Song" || artist != "Artist" {
		t.Fatalf("unexpected fallback metadata: track=%q artist=%q", track, artist)
	}
}

func TestStreamExpiry(t *testing.T) {
	tests := []struct {
		name string
		urls []string
		want int64
	}{
		{name: "query", urls: []string{"https://example.test/video?expire=200"}, want: 200},
		{name: "path", urls: []string{"https://manifest.googlevideo.com/api/manifest/expire/300/playlist.m3u8"}, want: 300},
		{name: "earliest across URLs", urls: []string{"https://example.test/expire/500/video", "https://example.test/audio?expire=400"}, want: 400},
		{name: "earliest within URL", urls: []string{"https://example.test/expire/700/video?expire=600"}, want: 600},
		{name: "invalid values ignored", urls: []string{"not a URL", "https://example.test/expire/nope/video?expire="}, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := streamExpiry(test.urls...); got != test.want {
				t.Fatalf("streamExpiry() = %d, want %d", got, test.want)
			}
		})
	}
}
