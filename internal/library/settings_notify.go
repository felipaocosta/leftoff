package library

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// Notifications returns the effective notification settings, unmasked,
// for sending (environment variables over settings.json).
func (s *Settings) Notifications() NotificationSettings {
	n := s.store.Settings().Notifications
	out := NotificationSettings{
		NtfyURL: n.NtfyURL, DiscordWebhook: n.DiscordWebhook,
		Telegram:    TelegramSettings{Token: n.Telegram.Token, ChatID: n.Telegram.ChatID},
		OnlyReading: n.OnlyReading, Language: n.Language,
	}
	if out.Language == "" {
		out.Language = LangPtBR
	}
	for _, f := range s.envFields(&out) {
		if *f.env != "" {
			*f.value = *f.env
			out.Locked = append(out.Locked, f.name)
		}
	}
	return out
}

// updateNotifications applies and validates the notification section.
func (s *Settings) updateNotifications(n *storage.Notifications, in NotificationSettings) error {
	n.NtfyURL = strings.TrimSpace(in.NtfyURL)
	n.DiscordWebhook = keepMasked(strings.TrimSpace(in.DiscordWebhook), n.DiscordWebhook)
	n.Telegram.Token = keepMasked(strings.TrimSpace(in.Telegram.Token), n.Telegram.Token)
	n.Telegram.ChatID = strings.TrimSpace(in.Telegram.ChatID)
	n.OnlyReading = in.OnlyReading
	n.Language = strings.TrimSpace(in.Language)
	s.restoreLocked(n)
	// Half of Telegram set by the environment just leaves Telegram off until
	// the other half is filled; it must not block saving other settings.
	partialOK := s.env.TelegramToken != "" || s.env.TelegramChatID != ""
	return validateNotifications(s.withEnv(*n), partialOK)
}

type envField struct {
	name       string
	env, value *string
}

func (s *Settings) envFields(n *NotificationSettings) []envField {
	return []envField{
		{FieldNtfyURL, &s.env.NtfyURL, &n.NtfyURL},
		{FieldDiscordWebhook, &s.env.DiscordWebhook, &n.DiscordWebhook},
		{FieldTelegramToken, &s.env.TelegramToken, &n.Telegram.Token},
		{FieldTelegramChatID, &s.env.TelegramChatID, &n.Telegram.ChatID},
	}
}

// restoreLocked puts back the file values of fields set by environment
// variables: the UI cannot change them.
func (s *Settings) restoreLocked(n *storage.Notifications) {
	saved := s.store.Settings().Notifications
	if s.env.NtfyURL != "" {
		n.NtfyURL = saved.NtfyURL
	}
	if s.env.DiscordWebhook != "" {
		n.DiscordWebhook = saved.DiscordWebhook
	}
	if s.env.TelegramToken != "" {
		n.Telegram.Token = saved.Telegram.Token
	}
	if s.env.TelegramChatID != "" {
		n.Telegram.ChatID = saved.Telegram.ChatID
	}
}

// withEnv is n with the environment variables on top, as it is used.
func (s *Settings) withEnv(n storage.Notifications) storage.Notifications {
	pairs := []struct {
		env   string
		value *string
	}{
		{s.env.NtfyURL, &n.NtfyURL}, {s.env.DiscordWebhook, &n.DiscordWebhook},
		{s.env.TelegramToken, &n.Telegram.Token}, {s.env.TelegramChatID, &n.Telegram.ChatID},
	}
	for _, p := range pairs {
		if p.env != "" {
			*p.value = p.env
		}
	}
	return n
}

func validateNotifications(n storage.Notifications, partialTelegramOK bool) error {
	var problems []error
	if n.NtfyURL != "" {
		if u, err := url.Parse(n.NtfyURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") ||
			u.Host == "" || strings.Trim(u.Path, "/") == "" {
			problems = append(problems, errors.New("ntfy_url must be http(s)://server/topic"))
		}
	}
	if n.DiscordWebhook != "" {
		if u, err := url.Parse(n.DiscordWebhook); err != nil || u.Scheme != "https" ||
			!discordHosts[u.Hostname()] || !strings.HasPrefix(u.Path, "/api/webhooks/") {
			problems = append(problems, errors.New("discord_webhook must be https://discord.com/api/webhooks/…"))
		}
	}
	if n.Language != "" && n.Language != LangPtBR && n.Language != LangEn {
		problems = append(problems, errors.New("language must be pt-BR or en"))
	}
	t := n.Telegram
	switch {
	case (t.Token == "") != (t.ChatID == "") && !partialTelegramOK:
		problems = append(problems, errors.New("telegram needs both token and chat_id"))
	case t.Token != "" && !telegramTokenRe.MatchString(t.Token):
		problems = append(problems, errors.New("telegram.token does not look like a bot token (123456:ABC…)"))
	case t.ChatID != "" && !telegramChatRe.MatchString(t.ChatID):
		problems = append(problems, errors.New("telegram.chat_id must be a number or @channel"))
	}
	if len(problems) > 0 {
		return fmt.Errorf("notifications: %w: %w", errors.Join(problems...), domain.ErrInvalidInput)
	}
	return nil
}

// mask hides a secret but its last 4 characters.
func mask(v string) string {
	if v == "" {
		return ""
	}
	if r := []rune(v); len(r) > 8 {
		return maskPrefix + string(r[len(r)-4:])
	}
	return maskPrefix
}

func keepMasked(in, stored string) string {
	if strings.HasPrefix(in, maskPrefix) {
		return stored
	}
	return in
}
