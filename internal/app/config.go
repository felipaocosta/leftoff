// Package app assembles the LeftOff server from its parts: it reads
// the configuration (environment variables), opens storage, builds the
// fetch client, the extraction pipeline, the library services, the
// scheduler and the API, and serves them with the embedded UI. The server
// binary, the desktop app (Wails) and the Android core all start it here.
//
// It does NOT hold business rules (library does) nor know how a front end
// shows the UI (a browser tab, a Wails window or an Android WebView).
package app

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/library"
)

// Config is read from environment variables (SPEC §6.3). settings.json
// support comes in a later phase.
type Config struct {
	DataDir     string        // DATA_DIR
	Port        int           // PORT, default 8080
	AuthToken   string        // AUTH_TOKEN, optional
	UserAgent   string        // USER_AGENT, optional
	MinInterval time.Duration // RATE_LIMIT_MS, default 2000
	LogLevel    slog.Level    // LOG_LEVEL: debug, info, warn, error

	// Anti-bot challenge solvers (RF-56).
	FlareSolverrURL string // FLARESOLVERR_URL, e.g. http://localhost:8191
	BrowserSolver   string // BROWSER_SOLVER: off, headless or interactive (app mode default)
	BrowserBin      string // BROWSER_BIN: browser executable; found automatically when empty

	// Language model for recipe generation (RF-54): any server with the
	// OpenAI-compatible API (Ollama, LM Studio). Disabled when empty.
	OllamaURL   string // OLLAMA_URL, e.g. http://localhost:11434
	OllamaModel string // OLLAMA_MODEL, required with OLLAMA_URL

	// New-chapter notifications (RF-26); they win over settings.json.
	Notify library.NotifyEnv // NTFY_URL, DISCORD_WEBHOOK, TELEGRAM_TOKEN, TELEGRAM_CHAT_ID

	// EnrichEnabled (ENRICH_ENABLED) turns AniList/MangaUpdates lookups on
	// or off and locks the setting; nil leaves it to settings.json (RF-57).
	EnrichEnabled *bool

	MangaDexLangs []string // MANGADEX_LANGS: chapter languages by preference, default pt-br,en

	CheckInterval  time.Duration // CHECK_INTERVAL_HOURS: periodic checks, default 6h; 0 disables
	CheckCompleted bool          // CHECK_COMPLETED: also check completed/dropped series
}

// LoadConfig reads the configuration through getenv (os.Getenv in main).
func LoadConfig(getenv func(string) string) (Config, error) {
	cfg := Config{
		DataDir:   getenv("DATA_DIR"),
		Port:      8080,
		AuthToken: getenv("AUTH_TOKEN"),
		UserAgent: getenv("USER_AGENT"),

		FlareSolverrURL: getenv("FLARESOLVERR_URL"),
		BrowserSolver:   getenv("BROWSER_SOLVER"),
		BrowserBin:      getenv("BROWSER_BIN"),
		OllamaURL:       strings.TrimSpace(getenv("OLLAMA_URL")),
		OllamaModel:     strings.TrimSpace(getenv("OLLAMA_MODEL")),
		Notify: library.NotifyEnv{
			NtfyURL:        strings.TrimSpace(getenv("NTFY_URL")),
			DiscordWebhook: strings.TrimSpace(getenv("DISCORD_WEBHOOK")),
			TelegramToken:  strings.TrimSpace(getenv("TELEGRAM_TOKEN")),
			TelegramChatID: strings.TrimSpace(getenv("TELEGRAM_CHAT_ID")),
		},
		CheckInterval: 6 * time.Hour,
	}
	for _, l := range strings.Split(getenv("MANGADEX_LANGS"), ",") {
		if l = strings.TrimSpace(strings.ToLower(l)); l != "" {
			cfg.MangaDexLangs = append(cfg.MangaDexLangs, l)
		}
	}
	switch cfg.BrowserSolver {
	case "", "off", "headless", "interactive":
	default:
		return cfg, fmt.Errorf("invalid BROWSER_SOLVER %q (off, headless or interactive)", cfg.BrowserSolver)
	}
	if err := checkOllama(cfg); err != nil {
		return cfg, err
	}
	if cfg.DataDir == "" {
		dir, err := defaultDataDir(getenv)
		if err != nil {
			return cfg, err
		}
		cfg.DataDir = dir
	}
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return cfg, fmt.Errorf("invalid LOG_LEVEL %q", v)
		}
	}
	return cfg, parseNumbers(&cfg, getenv)
}

// checkOllama requires a model with the URL and an http(s) URL.
func checkOllama(cfg Config) error {
	if cfg.OllamaURL == "" {
		return nil
	}
	if u, err := url.Parse(cfg.OllamaURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid OLLAMA_URL %q (e.g. http://localhost:11434)", cfg.OllamaURL)
	}
	if cfg.OllamaModel == "" {
		return fmt.Errorf("OLLAMA_URL is set but OLLAMA_MODEL is empty")
	}
	return nil
}

// parseNumbers reads the numeric and boolean variables.
func parseNumbers(cfg *Config, getenv func(string) string) error {
	if v := getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p < 0 || p > 65535 { // 0 picks a free port
			return fmt.Errorf("invalid PORT %q", v)
		}
		cfg.Port = p
	}
	if v := getenv("RATE_LIMIT_MS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err != nil || ms < 0 {
			return fmt.Errorf("invalid RATE_LIMIT_MS %q", v)
		}
		cfg.MinInterval = time.Duration(ms) * time.Millisecond
	}
	if v := getenv("CHECK_INTERVAL_HOURS"); v != "" {
		h, err := strconv.ParseFloat(v, 64)
		if err != nil || h < 0 {
			return fmt.Errorf("invalid CHECK_INTERVAL_HOURS %q", v)
		}
		cfg.CheckInterval = time.Duration(h * float64(time.Hour))
	}
	if v := getenv("ENRICH_ENABLED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid ENRICH_ENABLED %q", v)
		}
		cfg.EnrichEnabled = &b
	}
	if v := getenv("CHECK_COMPLETED"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("invalid CHECK_COMPLETED %q", v)
		}
		cfg.CheckCompleted = b
	}
	return nil
}

// defaultDataDir follows SPEC §6.1: %APPDATA% on Windows, otherwise
// $XDG_DATA_HOME or ~/.local/share. Docker sets DATA_DIR=/data explicitly.
func defaultDataDir(getenv func(string) string) (string, error) {
	var base string
	switch {
	case runtime.GOOS == "windows" && getenv("APPDATA") != "":
		base = getenv("APPDATA")
	case getenv("XDG_DATA_HOME") != "":
		base = getenv("XDG_DATA_HOME")
	case getenv("HOME") != "":
		base = filepath.Join(getenv("HOME"), ".local", "share")
	default:
		return "", fmt.Errorf("cannot find a data directory: set DATA_DIR")
	}
	return dataDirIn(base), nil
}

// legacyDataDir is the folder of the app before it was renamed LeftOff.
const legacyDataDir = "manga-tracker"

// dataDirIn is base/leftoff, or base/manga-tracker when only that one
// exists: a library from before the rename keeps being used where it is.
func dataDirIn(base string) string {
	current := filepath.Join(base, "leftoff")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	if legacy := filepath.Join(base, legacyDataDir); dirExists(legacy) {
		return legacy
	}
	return current
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
