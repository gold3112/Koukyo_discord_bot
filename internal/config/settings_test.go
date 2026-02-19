package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsManagerCloseIsIdempotent(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "settings.json")
	sm := NewSettingsManager(configPath)

	sm.Close()
	sm.Close()
}

func TestSettingsManagerCloseFlushesDirtyState(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "settings.json")
	sm := NewSettingsManager(configPath)

	channelID := "1234567890"
	settings := DefaultGuildSettings
	settings.NotificationChannel = &channelID
	sm.SetGuildSettings("guild-1", settings)

	sm.Close()

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	var saved map[string]GuildSettings
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("failed to parse saved settings: %v", err)
	}

	got, ok := saved["guild-1"]
	if !ok {
		t.Fatalf("guild settings were not persisted")
	}
	if got.NotificationChannel == nil || *got.NotificationChannel != channelID {
		t.Fatalf("notification channel not persisted: %+v", got.NotificationChannel)
	}
}
