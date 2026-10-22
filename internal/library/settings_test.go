package library

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/storage"
)

const (
	tgToken = "123456789:AAH-abcdefghijklmnopqrstuvwxyz012345"
	webhook = "https://discord.com/api/webhooks/111/secret-part-xyz9"
)

func newSettings(t *testing.T, env NotifyEnv) (*Settings, *storage.Store) {
	t.Helper()
	store, err := storage.Open(context.Background(), t.TempDir(), domain.FixedClock{T: t0}, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return NewSettings(store, env), store
}

func TestSettings_UpdateMasksAndKeepsSecrets(t *testing.T) {
	s, store := newSettings(t, NotifyEnv{})
	ctx := context.Background()
	in := SettingsUpdate{Notifications: &NotificationSettings{
		NtfyURL: " https://ntfy.sh/manga-x ", DiscordWebhook: webhook,
		Telegram: TelegramSettings{Token: tgToken, ChatID: "-100123"}, OnlyReading: true,
	}}
	view, err := s.Update(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	n := view.Notifications
	if n.NtfyURL != "https://ntfy.sh/manga-x" || n.DiscordWebhook != "••••xyz9" || n.Telegram.Token != "••••2345" ||
		n.Telegram.ChatID != "-100123" || !n.OnlyReading || n.Language != LangPtBR || len(n.Locked) != 0 {
		t.Errorf("view = %+v", n)
	}

	// The UI sends the masked values back: the secrets stay.
	view.Notifications.OnlyReading = false
	if _, err := s.Update(ctx, view.AsUpdate()); err != nil {
		t.Fatal(err)
	}
	got := store.Settings().Notifications
	if got.DiscordWebhook != webhook || got.Telegram.Token != tgToken || got.OnlyReading {
		t.Errorf("stored = %+v", got)
	}

	// An empty value turns a channel off.
	view.Notifications.DiscordWebhook = ""
	if _, err := s.Update(ctx, view.AsUpdate()); err != nil || store.Settings().Notifications.DiscordWebhook != "" {
		t.Errorf("clear webhook: %v", err)
	}
}

func TestSettings_EnvironmentWinsAndLocks(t *testing.T) {
	s, store := newSettings(t, NotifyEnv{NtfyURL: "https://ntfy.local/env", TelegramToken: tgToken})
	ctx := context.Background()
	// The token comes from the environment: a chat id alone is enough here.
	view, err := s.Update(ctx, SettingsUpdate{Notifications: &NotificationSettings{
		NtfyURL: "https://ntfy.sh/ignored", Telegram: TelegramSettings{Token: "", ChatID: "42"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	n := view.Notifications
	if n.NtfyURL != "https://ntfy.local/env" || strings.Join(n.Locked, ",") != "ntfy_url,telegram.token" {
		t.Errorf("view = %+v", n)
	}
	if got := store.Settings().Notifications; got.NtfyURL != "" || got.Telegram.ChatID != "42" {
		t.Errorf("locked fields must not be written: %+v", got)
	}
	eff := s.Notifications()
	if eff.Telegram.Token != tgToken || eff.Telegram.ChatID != "42" || eff.NtfyURL != "https://ntfy.local/env" {
		t.Errorf("effective = %+v", eff)
	}
}

func TestSettings_UpdateRejectsInvalidValues(t *testing.T) {
	s, store := newSettings(t, NotifyEnv{})
	tests := []struct {
		name string
		n    NotificationSettings
		want string
	}{
		{"ntfy without topic", NotificationSettings{NtfyURL: "https://ntfy.sh/"}, "ntfy_url"},
		{"ntfy not http", NotificationSettings{NtfyURL: "ftp://ntfy.sh/x"}, "ntfy_url"},
		{"discord other host", NotificationSettings{DiscordWebhook: "https://evil.example/api/webhooks/1/x"}, "discord_webhook"},
		{"discord http", NotificationSettings{DiscordWebhook: "http://discord.com/api/webhooks/1/x"}, "discord_webhook"},
		{"telegram token only", NotificationSettings{Telegram: TelegramSettings{Token: tgToken}}, "both"},
		{"telegram bad token", NotificationSettings{Telegram: TelegramSettings{Token: "abc", ChatID: "1"}}, "telegram.token"},
		{"telegram bad chat", NotificationSettings{Telegram: TelegramSettings{Token: tgToken, ChatID: "me"}}, "chat_id"},
		{"language", NotificationSettings{Language: "fr"}, "language"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Update(context.Background(), SettingsUpdate{Notifications: &tt.n})
			if !errors.Is(err, domain.ErrInvalidInput) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want invalid input about %s", err, tt.want)
			}
		})
	}
	if got := store.Settings().Notifications; got.NtfyURL != "" || got.DiscordWebhook != "" {
		t.Errorf("nothing should be saved: %+v", got)
	}
	if _, err := s.Update(context.Background(), SettingsUpdate{Notifications: &NotificationSettings{
		Telegram: TelegramSettings{Token: tgToken, ChatID: "@my_channel"},
	}}); err != nil {
		t.Errorf("@channel chat id: %v", err)
	}
}

func TestSettings_HalfOfTelegramFromTheEnvironmentDoesNotBlock(t *testing.T) {
	s, _ := newSettings(t, NotifyEnv{TelegramChatID: "42"})
	if _, err := s.Update(context.Background(), SettingsUpdate{Notifications: &NotificationSettings{NtfyURL: "https://ntfy.sh/x"}}); err != nil {
		t.Errorf("err = %v", err)
	}
}

func TestMask(t *testing.T) {
	for in, want := range map[string]string{"": "", "short": "••••", "abcdefghijk": "••••hijk"} {
		if got := mask(in); got != want {
			t.Errorf("mask(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSettings_DesktopSectionOnlyInTheDesktopApp(t *testing.T) {
	plain, store := newSettings(t, NotifyEnv{})
	ctx := context.Background()
	connected := &DesktopSettings{Mode: DesktopConnected, ServerURL: "http://truenas.local:8080/"}
	if v, err := plain.Update(ctx, SettingsUpdate{Desktop: connected}); err != nil || v.Desktop != nil || store.Settings().Desktop.Mode != "" {
		t.Errorf("server mode must ignore the desktop section: %+v, %v", v.Desktop, err)
	}

	desk := NewSettings(store, NotifyEnv{}).WithDesktop()
	if v := desk.View(); v.Desktop == nil || v.Desktop.Mode != DesktopStandalone {
		t.Errorf("default = %+v", v.Desktop)
	}
	v, err := desk.Update(ctx, SettingsUpdate{Desktop: connected})
	if err != nil || v.Desktop.Mode != DesktopConnected || v.Desktop.ServerURL != "http://truenas.local:8080" {
		t.Errorf("connected = %+v, %v", v.Desktop, err)
	}
	for name, d := range map[string]DesktopSettings{
		"bad mode":      {Mode: "remote"},
		"no url":        {Mode: DesktopConnected},
		"not http":      {Mode: DesktopConnected, ServerURL: "ftp://x"},
		"relative path": {Mode: DesktopConnected, ServerURL: "/library"},
	} {
		if _, err := desk.Update(ctx, SettingsUpdate{Desktop: &d}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if v, _ := desk.Update(ctx, SettingsUpdate{Desktop: &DesktopSettings{}}); v.Desktop.Mode != DesktopStandalone {
		t.Errorf("empty mode = %+v", v.Desktop)
	}
}
