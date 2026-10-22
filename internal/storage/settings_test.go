package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

func openSettingsStore(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(context.Background(), dir, domain.SystemClock{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSettings_DefaultsWithoutAFile(t *testing.T) {
	s := openSettingsStore(t, t.TempDir())
	if got := s.Settings(); got.Schema != SettingsSchema || got.Notifications.NtfyURL != "" || got.Notifications.OnlyReading {
		t.Errorf("defaults = %+v", got)
	}
}

// specSettings is the settings.json of SPEC §6.3 plus unknown fields at
// every level.
const specSettings = `{
  "schema": 1,
  "check_interval_hours": 6,
  "ollama": { "url": "", "model": "" },
  "notifications": {
    "ntfy_url": "https://ntfy.sh/manga-x",
    "discord_webhook": "",
    "telegram": { "token": "123:abc", "chat_id": "42", "silent": true },
    "quiet_hours": "23-7"
  },
  "ui": { "theme": "system", "view": "grid", "language": "pt-BR" }
}`

func TestSettings_SaveKeepsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, settingsFile), []byte(specSettings), 0o644); err != nil {
		t.Fatal(err)
	}
	s := openSettingsStore(t, dir)
	st := s.Settings()
	if st.Notifications.NtfyURL != "https://ntfy.sh/manga-x" || st.Notifications.Telegram.ChatID != "42" {
		t.Fatalf("loaded = %+v", st)
	}
	st.Notifications.DiscordWebhook = "https://discord.com/api/webhooks/1/x"
	st.Notifications.OnlyReading = true
	if err := s.SaveSettings(context.Background(), st); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, settingsFile))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	n := doc["notifications"].(map[string]any)
	if doc["check_interval_hours"] != 6.0 || doc["ui"] == nil || n["quiet_hours"] != "23-7" ||
		n["telegram"].(map[string]any)["silent"] != true || n["only_reading"] != true {
		t.Errorf("unknown fields lost or change missing: %s", data)
	}
	if again := openSettingsStore(t, dir).Settings(); again.Notifications.DiscordWebhook == "" {
		t.Error("saved settings were not reloaded")
	}
}

func TestSettings_RefusesNewerSchemaAndBadJSON(t *testing.T) {
	for name, content := range map[string]string{
		"newer schema": `{"schema": 2}`,
		"bad json":     `{"schema": `,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, settingsFile), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Open(context.Background(), dir, domain.SystemClock{}, Options{})
			if err == nil || !strings.Contains(err.Error(), settingsFile) {
				t.Errorf("err = %v", err)
			}
			if name == "newer schema" && !errors.Is(err, ErrNewerSchema) {
				t.Errorf("err = %v, want ErrNewerSchema", err)
			}
		})
	}
}

func TestSettings_SaveHonorsContext(t *testing.T) {
	s := openSettingsStore(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.SaveSettings(ctx, Settings{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}
