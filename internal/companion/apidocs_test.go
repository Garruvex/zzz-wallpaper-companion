package companion

import (
	"encoding/json"
	"github.com/Garruvex/zzz-wallpaper-companion/api"
	"github.com/Garruvex/zzz-wallpaper-companion/internal/gamepad"
	"github.com/Garruvex/zzz-wallpaper-companion/internal/lyrics"
	"github.com/Garruvex/zzz-wallpaper-companion/internal/systemstats"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverySettingsHaveNoExplorerLink(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	s.renderSettings(w, "port unavailable")
	if strings.Contains(w.Body.String(), "href=\"/docs\"") {
		t.Fatal("recovery listener cannot serve docs")
	}
}

func TestDocumentedTopLevelWireShapes(t *testing.T) {
	// Field presence and omissions only, not runtime schema validation.
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage
				Required   []string
			}
		}
	}
	if err := json.Unmarshal(api.Contract, &doc); err != nil {
		t.Fatal(err)
	}
	values := map[string]any{
		"lyrics.Request":       lyrics.LyricsRequest{},
		"lyrics.Response":      lyrics.LyricsResponse{},
		"lyrics.Line":          lyrics.LyricLine{},
		"system.Snapshot":      systemstats.SystemStatsSnapshot{},
		"youtube.Media":        resolvedMedia{},
		"youtube.PlaylistItem": playlistItem{},
		"input.Snapshot":       gamepad.Snapshot{},
		"dates.Document":       DateDocument{Version: 1, Events: []ImportantDate{}},
		"dates.Event":          ImportantDate{},
		"holidays.Settings":    HolidaySettings{},
		"holidays.Record":      HolidayRecord{},
		"settings.Document":    defaultSettings(),
		"games.Cart":           cart{},
		"games.Selection":      gameSelection{},
	}
	for name, value := range values {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		schema, ok := doc.Components.Schemas[name]
		if !ok {
			t.Fatal(name)
		}
		for field := range fields {
			if _, ok := schema.Properties[field]; !ok {
				t.Errorf("%s undocumented field %s", name, field)
			}
		}
		for _, field := range schema.Required {
			if _, ok := fields[field]; !ok {
				t.Errorf("%s required field absent %s", name, field)
			}
		}
	}
}
