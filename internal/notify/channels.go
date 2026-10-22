package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Service limits on the message text.
const (
	ntfyMaxBytes     = 3500 // ntfy accepts 4096 bytes
	ntfyMaxActions   = 3    // ntfy shows at most 3 buttons
	ntfyMaxLabel     = 40
	discordMaxBytes  = 1900 // Discord accepts 2000 characters
	telegramMaxBytes = 3900 // Telegram accepts 4096 characters
	telegramAPI      = "https://api.telegram.org"
)

// Ntfy publishes to a topic URL such as https://ntfy.sh/manga-x, with the
// JSON API (the title may have accents). Tapping opens the first link, and
// the first lines also get an "open" button each.
type Ntfy struct {
	topicURL string
	hc       *http.Client
}

// NewNtfy returns the ntfy channel for topicURL.
func NewNtfy(topicURL string, hc *http.Client) *Ntfy {
	return &Ntfy{topicURL: topicURL, hc: client(hc)}
}

// Name implements Channel.
func (*Ntfy) Name() string { return "ntfy" }

// Send implements Channel.
func (n *Ntfy) Send(ctx context.Context, m Message) error {
	u, err := url.Parse(n.topicURL)
	if err != nil {
		return fmt.Errorf("ntfy: invalid address")
	}
	path := strings.Trim(u.Path, "/")
	i := strings.LastIndex(path, "/")
	topic := path[i+1:]
	u.Path = "/" + path[:max(i, 0)] // the server root, which takes JSON
	body := map[string]any{
		"topic": topic, "title": m.Title, "tags": []string{"books"},
		"message": render(m, ntfyMaxBytes, nil), // the click target is the link
	}
	var actions []map[string]string
	for _, l := range m.Lines {
		if l.URL == "" || len(actions) == ntfyMaxActions {
			continue
		}
		if len(actions) == 0 {
			body["click"] = l.URL
		}
		actions = append(actions, map[string]string{"action": "view", "label": cut(l.Text, ntfyMaxLabel), "url": l.URL})
	}
	if len(actions) > 0 {
		body["actions"] = actions
	}
	return postJSON(ctx, n.hc, n.Name(), u.String(), body)
}

// Discord posts to a channel webhook.
type Discord struct {
	webhook string
	hc      *http.Client
}

// NewDiscord returns the Discord channel for a webhook URL.
func NewDiscord(webhook string, hc *http.Client) *Discord {
	return &Discord{webhook: webhook, hc: client(hc)}
}

// Name implements Channel.
func (*Discord) Name() string { return "discord" }

// Send implements Channel. Links go between <> so Discord does not add a
// preview for each one.
func (d *Discord) Send(ctx context.Context, m Message) error {
	text := "**" + m.Title + "**\n" + render(m, discordMaxBytes-len(m.Title), func(u string) string { return "<" + u + ">" })
	return postJSON(ctx, d.hc, d.Name(), d.webhook, map[string]any{
		"username": "LeftOff", "content": strings.TrimSpace(text),
		"allowed_mentions": map[string]any{"parse": []string{}}, // titles never ping anyone
	})
}

// Telegram sends with a bot to one chat, as plain text.
type Telegram struct {
	token, chatID, api string
	hc                 *http.Client
}

// NewTelegram returns the Telegram channel; api is the Bot API base URL
// ("" for the official one; tests use a local server).
func NewTelegram(token, chatID, api string, hc *http.Client) *Telegram {
	if api == "" {
		api = telegramAPI
	}
	return &Telegram{token: token, chatID: chatID, api: strings.TrimRight(api, "/"), hc: client(hc)}
}

// Name implements Channel.
func (*Telegram) Name() string { return "telegram" }

// Send implements Channel.
func (t *Telegram) Send(ctx context.Context, m Message) error {
	text := m.Title + "\n\n" + render(m, telegramMaxBytes-len(m.Title), func(u string) string { return u })
	return postJSON(ctx, t.hc, t.Name(), t.api+"/bot"+t.token+"/sendMessage", map[string]any{
		"chat_id": t.chatID, "text": strings.TrimSpace(text), "disable_web_page_preview": true,
	})
}

// cut shortens s to n characters with an ellipsis.
func cut(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
