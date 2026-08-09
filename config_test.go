package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigStoreMigratesTranscodeHeight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"port":8765,"maxHeight":720,"updateChannel":"nightly"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := newConfigStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get().TranscodeHeight; got != 360 {
		t.Fatalf("TranscodeHeight = %d, want 360", got)
	}
	if !store.Get().AutoUpdate {
		t.Fatal("AutoUpdate should default to true for existing settings")
	}
}

func TestConfigStoreDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := newConfigStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get(); got.Port != defaultPort || got.UpdateChannel != "nightly" || got.MaxHeight != 720 || got.TranscodeHeight != 360 || !got.AutoUpdate {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	next := Settings{Port: 9000, MaxHeight: 1080, UpdateChannel: "stable", TranscodeHeight: 480, AutoUpdate: false}
	if err := store.Save(next); err != nil {
		t.Fatal(err)
	}
	reloaded, err := newConfigStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get(); got != next {
		t.Fatalf("got %+v, want %+v", got, next)
	}
}

func TestSettingsValidation(t *testing.T) {
	tests := []Settings{
		{Port: 80, MaxHeight: 720, UpdateChannel: "nightly", TranscodeHeight: 360},
		{Port: 8765, MaxHeight: 999, UpdateChannel: "nightly", TranscodeHeight: 360},
		{Port: 8765, MaxHeight: 720, UpdateChannel: "unknown", TranscodeHeight: 360},
		{Port: 8765, MaxHeight: 720, UpdateChannel: "nightly", TranscodeHeight: 1440},
	}
	for _, value := range tests {
		if validateSettings(value) == nil {
			t.Fatalf("expected validation error for %+v", value)
		}
	}
}
