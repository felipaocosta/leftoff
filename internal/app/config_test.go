package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/fetch/browser"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(env(map[string]string{
		"DATA_DIR": "/data", "PORT": "9090", "AUTH_TOKEN": "t", "RATE_LIMIT_MS": "500", "LOG_LEVEL": "debug",
		"MANGADEX_LANGS": " EN , ja ,,", "CHECK_INTERVAL_HOURS": "0.5", "CHECK_COMPLETED": "true",
		"OLLAMA_URL": "http://localhost:1234/v1", "OLLAMA_MODEL": " qwen ",
		"NTFY_URL": " https://ntfy.sh/x ", "TELEGRAM_CHAT_ID": "42", "ENRICH_ENABLED": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/data" || cfg.Port != 9090 || cfg.AuthToken != "t" ||
		cfg.MinInterval != 500*time.Millisecond || cfg.LogLevel != slog.LevelDebug ||
		strings.Join(cfg.MangaDexLangs, ",") != "en,ja" || cfg.CheckInterval != 30*time.Minute || !cfg.CheckCompleted ||
		cfg.OllamaURL != "http://localhost:1234/v1" || cfg.OllamaModel != "qwen" ||
		cfg.Notify.NtfyURL != "https://ntfy.sh/x" || cfg.Notify.TelegramChatID != "42" ||
		cfg.EnrichEnabled == nil || !*cfg.EnrichEnabled {
		t.Errorf("cfg = %+v", cfg)
	}

	cfg, err = LoadConfig(env(map[string]string{"HOME": "/home/u"}))
	if err != nil || cfg.Port != 8080 || cfg.CheckInterval != 6*time.Hour || cfg.DataDir != filepath.Join("/home/u", ".local", "share", "leftoff") {
		t.Errorf("defaults: %+v %v", cfg, err)
	}
	cfg, _ = LoadConfig(env(map[string]string{"XDG_DATA_HOME": "/xdg", "HOME": "/home/u"}))
	if cfg.DataDir != filepath.Join("/xdg", "leftoff") {
		t.Errorf("xdg: %q", cfg.DataDir)
	}

	for _, bad := range []map[string]string{
		{"DATA_DIR": "/d", "PORT": "http"},
		{"DATA_DIR": "/d", "PORT": "70000"},
		{"DATA_DIR": "/d", "RATE_LIMIT_MS": "-1"},
		{"DATA_DIR": "/d", "LOG_LEVEL": "loud"},
		{"DATA_DIR": "/d", "BROWSER_SOLVER": "always"},
		{"DATA_DIR": "/d", "CHECK_INTERVAL_HOURS": "soon"},
		{"DATA_DIR": "/d", "CHECK_COMPLETED": "maybe"},
		{"DATA_DIR": "/d", "ENRICH_ENABLED": "sometimes"},
		{"DATA_DIR": "/d", "OLLAMA_URL": "http://localhost:11434"}, // no model
		{"DATA_DIR": "/d", "OLLAMA_URL": "localhost:11434", "OLLAMA_MODEL": "m"},
		{}, // no DATA_DIR and no HOME
	} {
		if _, err := LoadConfig(env(bad)); err == nil {
			t.Errorf("%v: expected error", bad)
		}
	}
}

func TestNewSolvers(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	if got, r := newSolvers(Config{}, logger); len(got) != 0 || r != nil {
		t.Errorf("no config should mean no solvers, got %d %v", len(got), r)
	}
	got, renderer := newSolvers(Config{FlareSolverrURL: "http://fs:8191", BrowserSolver: "interactive", DataDir: t.TempDir()}, logger)
	if browser.Available() {
		if len(got) != 2 || got[0].Name() != "flaresolverr" || got[1].Name() != "browser" || renderer == nil {
			t.Errorf("solvers = %v, renderer = %v", got, renderer)
		}
	} else if len(got) != 1 || renderer != nil {
		t.Errorf("without rod only FlareSolverr is available: %v %v", got, renderer)
	}
}

// A library from before the rename keeps its folder; a new install, or one
// that already has leftoff/, uses leftoff/.
func TestDataDirIn_KeepsTheFolderFromBeforeTheRename(t *testing.T) {
	base := t.TempDir()
	if got := dataDirIn(base); got != filepath.Join(base, "leftoff") {
		t.Errorf("new install = %s", got)
	}
	if err := os.Mkdir(filepath.Join(base, "manga-tracker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := dataDirIn(base); got != filepath.Join(base, "manga-tracker") {
		t.Errorf("with the old folder = %s", got)
	}
	if err := os.Mkdir(filepath.Join(base, "leftoff"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := dataDirIn(base); got != filepath.Join(base, "leftoff") {
		t.Errorf("with both = %s", got)
	}
}
