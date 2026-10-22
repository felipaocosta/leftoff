package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/notify"
)

// maxPending caps the lines kept for a channel that keeps failing.
const maxPending = 50

// ErrNoChannels means no notification channel is configured.
var ErrNoChannels = errors.New("no notification channel is configured")

// messages are the texts of the notifications, per language.
var messages = map[string]struct {
	title, titleN, one, many, count, test string
}{
	LangPtBR: {
		title: "LeftOff: capítulo novo", titleN: "LeftOff: %d obras com capítulos novos",
		one: "%s: cap. %s", many: "%s: caps. %s–%s (%d novos)", count: "%s: %d capítulos novos",
		test: "Mensagem de teste: as notificações estão funcionando.",
	},
	LangEn: {
		title: "LeftOff: new chapter", titleN: "LeftOff: %d series with new chapters",
		one: "%s: ch. %s", many: "%s: chs. %s–%s (%d new)", count: "%s: %d new chapters",
		test: "Test message: notifications are working.",
	},
}

// ChannelResult is the outcome of a test message on one channel.
type ChannelResult struct {
	Channel string `json:"channel"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// Announcer sends one message per periodic check with the new chapters it
// found (RF-26), to every configured channel. Lines that a channel failed
// to receive are sent again with the next message (kept in memory).
type Announcer struct {
	lib      *Service
	settings *Settings
	hc       *http.Client
	log      *slog.Logger
	// telegramAPI replaces the Bot API base URL in tests.
	telegramAPI string

	mu      sync.Mutex
	pending map[string][]notify.Line // channel name → lines not delivered yet
}

// NewAnnouncer builds the announcer; hc bounds each request.
func NewAnnouncer(lib *Service, settings *Settings, hc *http.Client, logger *slog.Logger) *Announcer {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Announcer{lib: lib, settings: settings, hc: hc, log: logger, pending: map[string][]notify.Line{}}
}

// Announce sends the new chapters of a check. Failures are logged (never
// with the secret URL) and do not affect the library.
func (a *Announcer) Announce(ctx context.Context, reports []RefreshReport) {
	cfg := a.settings.Notifications()
	lines := a.lines(reports, cfg)
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, ch := range a.channels(cfg) {
		name := ch.Name()
		all := slices.Concat(a.pending[name], lines)
		if len(all) == 0 {
			continue
		}
		if err := ch.Send(ctx, message(cfg.Language, all)); err != nil {
			a.log.WarnContext(ctx, "notification failed; will retry with the next one", "channel", name, "err", err)
			a.pending[name] = all[max(0, len(all)-maxPending):]
			continue
		}
		delete(a.pending, name)
	}
}

// Test sends a test message to every configured channel.
func (a *Announcer) Test(ctx context.Context) ([]ChannelResult, error) {
	cfg := a.settings.Notifications()
	channels := a.channels(cfg)
	if len(channels) == 0 {
		return nil, fmt.Errorf("%w: %w", ErrNoChannels, domain.ErrInvalidInput)
	}
	results := make([]ChannelResult, 0, len(channels))
	for _, ch := range channels {
		r := ChannelResult{Channel: ch.Name(), OK: true}
		if err := ch.Send(ctx, notify.Message{Title: "LeftOff", Text: texts(cfg.Language).test}); err != nil {
			r.OK, r.Error = false, err.Error()
		}
		results = append(results, r)
	}
	return results, nil
}

func (a *Announcer) channels(cfg NotificationSettings) []notify.Channel {
	var out []notify.Channel
	if cfg.NtfyURL != "" {
		out = append(out, notify.NewNtfy(cfg.NtfyURL, a.hc))
	}
	if cfg.DiscordWebhook != "" {
		out = append(out, notify.NewDiscord(cfg.DiscordWebhook, a.hc))
	}
	if cfg.Telegram.Token != "" && cfg.Telegram.ChatID != "" {
		out = append(out, notify.NewTelegram(cfg.Telegram.Token, cfg.Telegram.ChatID, a.telegramAPI, a.hc))
	}
	return out
}

// lines turns the reports with new chapters into one line per series,
// skipping series outside "reading" when only_reading is on.
func (a *Announcer) lines(reports []RefreshReport, cfg NotificationSettings) []notify.Line {
	var out []notify.Line
	for _, r := range reports {
		if len(r.Added) == 0 {
			continue
		}
		sr, err := a.lib.Get(r.SeriesID)
		if err != nil || (cfg.OnlyReading && sr.Category != domain.CategoryReading) {
			continue // removed meanwhile, or filtered out
		}
		out = append(out, line(cfg.Language, sr.Title, r.Added))
	}
	return out
}

// line describes the new chapters of one series and links the newest.
func line(lang, title string, added []domain.Chapter) notify.Line {
	m := texts(lang)
	newest, lo, hi := added[0], (*float64)(nil), (*float64)(nil)
	for i, c := range added {
		if c.Number == nil {
			continue
		}
		if lo == nil || *c.Number < *lo {
			lo = added[i].Number
		}
		if hi == nil || *c.Number > *hi {
			hi, newest = added[i].Number, c
		}
	}
	var text string
	switch {
	case len(added) == 1 && hi != nil:
		text = fmt.Sprintf(m.one, title, number(lang, *hi))
	case len(added) == 1:
		text = fmt.Sprintf(m.one, title, newest.Label)
	case lo != nil && *lo != *hi:
		text = fmt.Sprintf(m.many, title, number(lang, *lo), number(lang, *hi), len(added))
	default:
		text = fmt.Sprintf(m.count, title, len(added))
	}
	return notify.Line{Text: text, URL: newest.URL}
}

func message(lang string, lines []notify.Line) notify.Message {
	m := texts(lang)
	title := m.title
	if len(lines) > 1 {
		title = fmt.Sprintf(m.titleN, len(lines))
	}
	return notify.Message{Title: title, Lines: lines}
}

func texts(lang string) struct{ title, titleN, one, many, count, test string } {
	if m, ok := messages[lang]; ok {
		return m
	}
	return messages[LangPtBR]
}

// number formats a chapter number: "40.5" in English, "40,5" in Portuguese.
func number(lang string, n float64) string {
	s := strconv.FormatFloat(n, 'f', -1, 64)
	if lang == LangPtBR {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}
