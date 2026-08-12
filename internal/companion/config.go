package companion

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const defaultPort = 8765

type Settings struct {
	Port            int    `json:"port"`
	MaxHeight       int    `json:"maxHeight"`
	UpdateChannel   string `json:"updateChannel"`
	LaunchOnStartup bool   `json:"launchOnStartup"`
	TranscodeHeight int    `json:"transcodeHeight"`
	AutoUpdate      bool   `json:"autoUpdate"`
}

type ConfigStore struct {
	mu   sync.RWMutex
	path string
	data Settings
}

func defaultSettings() Settings {
	return Settings{Port: defaultPort, MaxHeight: 720, UpdateChannel: "nightly", TranscodeHeight: 360, AutoUpdate: true}
}

func newConfigStore(path string) (*ConfigStore, error) {
	s := &ConfigStore{path: path, data: defaultSettings()}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, s.Save(s.data)
	}
	if err != nil {
		return nil, err
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(body, &stored); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &s.data); err != nil {
		return nil, err
	}
	// Migrate settings written before live transcoding was introduced.
	if s.data.TranscodeHeight == 0 {
		s.data.TranscodeHeight = 360
	}
	if _, exists := stored["autoUpdate"]; !exists {
		s.data.AutoUpdate = true
	}
	if err := validateSettings(s.data); err != nil {
		return nil, err
	}
	return s, nil
}

func validateSettings(v Settings) error {
	if v.Port < 1024 || v.Port > 65535 {
		return errors.New("port must be between 1024 and 65535")
	}
	if v.MaxHeight != 360 && v.MaxHeight != 480 && v.MaxHeight != 720 && v.MaxHeight != 1080 {
		return errors.New("maxHeight must be 360, 480, 720, or 1080")
	}
	if v.UpdateChannel != "stable" && v.UpdateChannel != "nightly" {
		return errors.New("updateChannel must be stable or nightly")
	}
	if v.TranscodeHeight != 240 && v.TranscodeHeight != 360 && v.TranscodeHeight != 480 && v.TranscodeHeight != 720 && v.TranscodeHeight != 1080 {
		return errors.New("transcodeHeight must be 240, 360, 480, 720, or 1080")
	}
	return nil
}

func (s *ConfigStore) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

func (s *ConfigStore) Save(v Settings) error {
	if err := validateSettings(v); err != nil {
		return err
	}
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.path, append(body, '\n'), 0o600); err != nil {
		return err
	}
	s.mu.Lock()
	s.data = v
	s.mu.Unlock()
	return nil
}
