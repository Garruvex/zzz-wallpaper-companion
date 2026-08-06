package main

import (
	"path/filepath"
	"testing"
)

func TestConfigStoreDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := newConfigStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Get(); got.Port != defaultPort || got.UpdateChannel != "nightly" || got.MaxHeight != 720 {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	next := Settings{Port: 9000, MaxHeight: 1080, UpdateChannel: "stable"}
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
		{Port: 80, MaxHeight: 720, UpdateChannel: "nightly"},
		{Port: 8765, MaxHeight: 999, UpdateChannel: "nightly"},
		{Port: 8765, MaxHeight: 720, UpdateChannel: "unknown"},
	}
	for _, value := range tests {
		if validateSettings(value) == nil {
			t.Fatalf("expected validation error for %+v", value)
		}
	}
}
