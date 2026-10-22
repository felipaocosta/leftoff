package library

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// SettingsStore keeps settings.json (implemented by *storage.Store).
type SettingsStore interface {
	Settings() storage.Settings
	SaveSettings(ctx context.Context, s storage.Settings) error
}

// NotifyEnv are the notification settings given by environment variables
// (NTFY_URL, DISCORD_WEBHOOK, TELEGRAM_TOKEN, TELEGRAM_CHAT_ID). A
// non-empty one wins over settings.json and cannot be changed in the UI.
type NotifyEnv struct {
	NtfyURL, DiscordWebhook, TelegramToken, TelegramChatID string
}

// Notification setting names, as used in Locked.
const (
	FieldNtfyURL        = "ntfy_url"
	FieldDiscordWebhook = "discord_webhook"
	FieldTelegramToken  = "telegram.token"
	FieldTelegramChatID = "telegram.chat_id"
)

// Languages of the notification messages.
const (
	LangPtBR = "pt-BR"
	LangEn   = "en"
)

// maskPrefix starts a masked secret. The UI sends masked values back
// unchanged, which keeps the stored secret.
const maskPrefix = "••••"

var (
	telegramTokenRe = regexp.MustCompile(`^\d{5,}:[A-Za-z0-9_-]{30,}$`)
	telegramChatRe  = regexp.MustCompile(`^(-?\d+|@[A-Za-z0-9_]{5,})$`)
	discordHosts    = map[string]bool{"discord.com": true, "discordapp.com": true, "ptb.discord.com": true, "canary.discord.com": true}
)

// NotificationSettings are the notification settings as the API shows and
// takes them. Shown, the Discord webhook and the Telegram token are masked
// and Locked lists the fields set by environment variables.
type NotificationSettings struct {
	NtfyURL        string           `json:"ntfy_url"`
	DiscordWebhook string           `json:"discord_webhook"`
	Telegram       TelegramSettings `json:"telegram"`
	OnlyReading    bool             `json:"only_reading"`
	Language       string           `json:"language"` // of the messages: pt-BR or en
	Locked         []string         `json:"locked,omitempty"`
}

// TelegramSettings are the bot token and the chat id.
type TelegramSettings struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
}

// EnrichmentSettings turns AniList/MangaUpdates lookups on (RF-57).
// Locked means ENRICH_ENABLED decides it.
type EnrichmentSettings struct {
	Enabled bool `json:"enabled"`
	Locked  bool `json:"locked,omitempty"`
}

// Desktop app modes (Phase 6).
const (
	DesktopStandalone = "standalone"
	DesktopConnected  = "connected"
)

// DesktopSettings is how the desktop app opens: its own library or a
// server's UI at ServerURL.
type DesktopSettings struct {
	Mode      string `json:"mode"`
	ServerURL string `json:"server_url"`
}

// SettingsView is the body of GET /settings and the answer of PUT.
// Desktop is only present in the desktop app.
type SettingsView struct {
	Notifications NotificationSettings `json:"notifications"`
	Enrichment    EnrichmentSettings   `json:"enrichment"`
	Desktop       *DesktopSettings     `json:"desktop,omitempty"`
}

// SettingsUpdate is the body of PUT /settings: a missing section is left
// as it is.
type SettingsUpdate struct {
	Notifications *NotificationSettings `json:"notifications"`
	Enrichment    *EnrichmentSettings   `json:"enrichment"`
	Desktop       *DesktopSettings      `json:"desktop"`
}

// AsUpdate sends every section of v back.
func (v SettingsView) AsUpdate() SettingsUpdate {
	return SettingsUpdate{Notifications: &v.Notifications, Enrichment: &v.Enrichment, Desktop: v.Desktop}
}

// Settings reads and changes the user settings (settings.json), with the
// environment variables on top (SPEC §6.3).
type Settings struct {
	store     SettingsStore
	env       NotifyEnv
	enrichEnv *bool // ENRICH_ENABLED; nil when unset
	desktop   bool  // the desktop app: show and accept the desktop section
}

// WithDesktop shows the desktop section (the desktop app only).
func (s *Settings) WithDesktop() *Settings {
	s.desktop = true
	return s
}

// Desktop returns how the desktop app opens; the mode defaults to
// standalone.
func (s *Settings) Desktop() DesktopSettings {
	d := s.store.Settings().Desktop
	if d.Mode != DesktopConnected {
		d.Mode = DesktopStandalone
	}
	return DesktopSettings{Mode: d.Mode, ServerURL: d.ServerURL}
}

// WithEnrichEnv makes ENRICH_ENABLED decide enrichment (and lock it).
func (s *Settings) WithEnrichEnv(enabled *bool) *Settings {
	s.enrichEnv = enabled
	return s
}

// Enrichment returns the effective enrichment setting.
func (s *Settings) Enrichment() EnrichmentSettings {
	if s.enrichEnv != nil {
		return EnrichmentSettings{Enabled: *s.enrichEnv, Locked: true}
	}
	return EnrichmentSettings{Enabled: s.store.Settings().Enrichment.Enabled}
}

// NewSettings builds the settings service.
func NewSettings(store SettingsStore, env NotifyEnv) *Settings {
	return &Settings{store: store, env: env}
}

// View returns the settings for the UI, with secrets masked.
func (s *Settings) View() SettingsView {
	n := s.Notifications()
	n.DiscordWebhook = mask(n.DiscordWebhook)
	n.Telegram.Token = mask(n.Telegram.Token)
	v := SettingsView{Notifications: n, Enrichment: s.Enrichment()}
	if s.desktop {
		d := s.Desktop()
		v.Desktop = &d
	}
	return v
}

// Update validates and saves the notification settings. Masked secrets
// keep the stored value and fields locked by environment variables are
// left as they are in the file.
func (s *Settings) Update(ctx context.Context, u SettingsUpdate) (SettingsView, error) {
	cur := s.store.Settings()
	if u.Enrichment != nil && s.enrichEnv == nil {
		cur.Enrichment.Enabled = u.Enrichment.Enabled
	}
	if u.Notifications != nil {
		if err := s.updateNotifications(&cur.Notifications, *u.Notifications); err != nil {
			return SettingsView{}, err
		}
	}
	if u.Desktop != nil && s.desktop {
		d, err := validDesktop(*u.Desktop)
		if err != nil {
			return SettingsView{}, err
		}
		cur.Desktop.Mode, cur.Desktop.ServerURL = d.Mode, d.ServerURL
	}
	if err := s.store.SaveSettings(ctx, cur); err != nil {
		return SettingsView{}, err
	}
	return s.View(), nil
}

// validDesktop checks the mode and, in connected mode, the server URL.
func validDesktop(d DesktopSettings) (DesktopSettings, error) {
	d.ServerURL = strings.TrimRight(strings.TrimSpace(d.ServerURL), "/")
	switch d.Mode {
	case "", DesktopStandalone:
		d.Mode = DesktopStandalone
	case DesktopConnected:
		u, err := url.Parse(d.ServerURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return d, fmt.Errorf("desktop.server_url must be http(s)://host[:port]: %w", domain.ErrInvalidInput)
		}
	default:
		return d, fmt.Errorf("desktop.mode must be standalone or connected: %w", domain.ErrInvalidInput)
	}
	return d, nil
}
