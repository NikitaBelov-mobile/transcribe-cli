package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Settings stores user defaults for the CLI.
type Settings struct {
	DefaultModel string `json:"defaultModel"`
}

func LoadSettings(path string) (Settings, error) {
	payload, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}

	var settings Settings
	if err := json.Unmarshal(payload, &settings); err != nil {
		return Settings{}, err
	}
	settings.DefaultModel = strings.TrimSpace(settings.DefaultModel)
	return settings, nil
}

func SaveSettings(path string, settings Settings) error {
	settings.DefaultModel = strings.TrimSpace(settings.DefaultModel)
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
