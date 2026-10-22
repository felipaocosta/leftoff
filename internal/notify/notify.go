// Package notify sends short text messages to ntfy, Discord and Telegram
// (RF-26). Each Channel knows one service's API and its size limits.
//
// It does NOT decide what to announce or when (library and scheduler do),
// nor store any state. Errors never include the request URL, because the
// Telegram token and the Discord webhook are part of it.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// maxErrorBody is how much of a failed answer is kept in the error.
const maxErrorBody = 200

// Message is a title, an optional text and lines; each line may link to a
// page.
type Message struct {
	Title string
	Text  string
	Lines []Line
}

// Line is one item of a message, e.g. "Eleceed: ch. 424".
type Line struct {
	Text string
	URL  string
}

// Channel delivers messages to one service.
type Channel interface {
	Name() string // "ntfy", "discord" or "telegram"
	Send(ctx context.Context, m Message) error
}

// postJSON sends body as JSON to endpoint and fails on a non-2xx answer.
// The error names the channel but never the endpoint (it holds secrets).
func postJSON(ctx context.Context, hc *http.Client, channel, endpoint string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%s: encode message: %w", channel, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: invalid address", channel)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err // drop the URL, which holds the secret
		}
		return fmt.Errorf("%s: send: %w", channel, err)
	}
	defer func() { _ = resp.Body.Close() }() // read-only body
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody)) // best effort detail
		return fmt.Errorf("%s: HTTP %d: %s", channel, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}
	_, _ = io.Copy(io.Discard, resp.Body) // let the connection be reused
	return nil
}

// render writes the lines as text, one per line, each followed by its URL
// formatted by link (nil leaves URLs out), and stops before limit bytes
// with a "… +N" line.
func render(m Message, limit int, link func(string) string) string {
	var b strings.Builder
	b.WriteString(m.Text)
	for i, l := range m.Lines {
		line := "• " + l.Text
		if l.URL != "" && link != nil {
			line += " " + link(l.URL)
		}
		more := fmt.Sprintf("… +%d", len(m.Lines)-i)
		if b.Len()+len(line)+len(more)+2 > limit {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(more)
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

func client(hc *http.Client) *http.Client {
	if hc == nil {
		return http.DefaultClient
	}
	return hc
}
