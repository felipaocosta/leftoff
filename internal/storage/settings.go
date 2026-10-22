package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/felipaocosta/leftoff/internal/domain"
)

const settingsFile = "settings.json"

// SettingsSchema is the settings.json schema version written by this build.
const SettingsSchema = 1

// Settings is the root of settings.json (SPEC §6.3). Only the fields the
// app reads from the file are typed; every other field (and any unknown
// one) is kept in Extra and written back unchanged.
type Settings struct {
	Schema        int           `json:"schema"`
	Notifications Notifications `json:"notifications"`
	Enrichment    Enrichment    `json:"enrichment"`
	Desktop       Desktop       `json:"desktop"`
	Extra         domain.Extra  `json:"-"`
}

// Desktop is how the desktop app opens (Phase 6): its own library
// ("standalone", the default) or a server's ("connected").
type Desktop struct {
	Mode      string       `json:"mode"`       // "standalone" (or "") or "connected"
	ServerURL string       `json:"server_url"` // used in connected mode
	Extra     domain.Extra `json:"-"`
}

// MarshalJSON encodes the desktop settings and their unknown fields.
func (d Desktop) MarshalJSON() ([]byte, error) {
	type plain Desktop
	return domain.MarshalWithExtra(plain(d), d.Extra)
}

// UnmarshalJSON decodes the desktop settings and keeps unknown fields.
func (d *Desktop) UnmarshalJSON(b []byte) error {
	type plain Desktop
	var p plain
	extra, err := domain.UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*d = Desktop(p)
	d.Extra = extra
	return nil
}

// Enrichment turns on looking series up in AniList and MangaUpdates
// (RF-57). Off by default: the titles leave the local network.
type Enrichment struct {
	Enabled bool         `json:"enabled"`
	Extra   domain.Extra `json:"-"`
}

// MarshalJSON encodes the enrichment settings and their unknown fields.
func (e Enrichment) MarshalJSON() ([]byte, error) {
	type plain Enrichment
	return domain.MarshalWithExtra(plain(e), e.Extra)
}

// UnmarshalJSON decodes the enrichment settings and keeps unknown fields.
func (e *Enrichment) UnmarshalJSON(b []byte) error {
	type plain Enrichment
	var p plain
	extra, err := domain.UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*e = Enrichment(p)
	e.Extra = extra
	return nil
}

// Notifications configures the new-chapter messages (RF-26). Empty values
// disable a channel.
type Notifications struct {
	NtfyURL        string       `json:"ntfy_url"`
	DiscordWebhook string       `json:"discord_webhook"`
	Telegram       Telegram     `json:"telegram"`
	OnlyReading    bool         `json:"only_reading"` // only series in the "reading" category
	Language       string       `json:"language"`     // of the messages: "pt-BR" (default) or "en"
	Extra          domain.Extra `json:"-"`
}

// Telegram is a bot token and the chat that receives the messages.
type Telegram struct {
	Token  string       `json:"token"`
	ChatID string       `json:"chat_id"`
	Extra  domain.Extra `json:"-"`
}

// MarshalJSON encodes the settings and their unknown fields.
func (s Settings) MarshalJSON() ([]byte, error) {
	type plain Settings
	return domain.MarshalWithExtra(plain(s), s.Extra)
}

// UnmarshalJSON decodes the settings and keeps unknown fields in Extra.
func (s *Settings) UnmarshalJSON(b []byte) error {
	type plain Settings
	var p plain
	extra, err := domain.UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*s = Settings(p)
	s.Extra = extra
	return nil
}

// MarshalJSON encodes the notification settings and their unknown fields.
func (n Notifications) MarshalJSON() ([]byte, error) {
	type plain Notifications
	return domain.MarshalWithExtra(plain(n), n.Extra)
}

// UnmarshalJSON decodes the notification settings and keeps unknown fields.
func (n *Notifications) UnmarshalJSON(b []byte) error {
	type plain Notifications
	var p plain
	extra, err := domain.UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*n = Notifications(p)
	n.Extra = extra
	return nil
}

// MarshalJSON encodes the Telegram settings and their unknown fields.
func (t Telegram) MarshalJSON() ([]byte, error) {
	type plain Telegram
	return domain.MarshalWithExtra(plain(t), t.Extra)
}

// UnmarshalJSON decodes the Telegram settings and keeps unknown fields.
func (t *Telegram) UnmarshalJSON(b []byte) error {
	type plain Telegram
	var p plain
	extra, err := domain.UnmarshalWithExtra(b, &p)
	if err != nil {
		return err
	}
	*t = Telegram(p)
	t.Extra = extra
	return nil
}

// Settings returns the current settings (a copy). A missing settings.json
// reads as the defaults.
func (s *Store) Settings() Settings { return *s.settings.Load() }

// SaveSettings writes settings.json atomically and makes it current. The
// caller validates the values; storage keeps the unknown fields it was given.
func (s *Store) SaveSettings(ctx context.Context, next Settings) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	next.Schema = SettingsSchema
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := writeFileAtomic(s.settingsPath(), append(data, '\n'), filePerm); err != nil {
		return err
	}
	s.settings.Store(&next)
	return nil
}

// loadSettings reads settings.json; a missing file yields the defaults. A
// newer schema is refused so an older build never drops its fields.
func (s *Store) loadSettings() (*Settings, error) {
	data, err := os.ReadFile(s.settingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return &Settings{Schema: SettingsSchema}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.settingsPath(), err)
	}
	var st Settings
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("decode %s: %w", s.settingsPath(), err)
	}
	if st.Schema > SettingsSchema {
		return nil, fmt.Errorf("%s has schema %d: %w", s.settingsPath(), st.Schema, ErrNewerSchema)
	}
	st.Schema = SettingsSchema
	return &st, nil
}

func (s *Store) settingsPath() string { return filepath.Join(s.dir, settingsFile) }
