package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recorder is a fake service that keeps the last request.
type recorder struct {
	path   string
	body   map[string]any
	status int
}

func (r *recorder) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.path = req.URL.Path
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content type %q", req.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(req.Body).Decode(&r.body); err != nil {
			t.Error(err)
		}
		if r.status != 0 {
			w.WriteHeader(r.status)
			_, _ = w.Write([]byte(`{"description":"Unauthorized"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

var msg = Message{
	Title: "LeftOff: 2 obras com capítulos novos",
	Lines: []Line{
		{Text: "Eleceed: cap. 424", URL: "https://www.mangaread.org/manga/eleceed/chapter-424/"},
		{Text: "Solo: caps. 9–10 (2 novos)", URL: "https://site.org/solo/10"},
	},
}

func TestNtfy_PublishesJSONToTheServerRoot(t *testing.T) {
	r := &recorder{}
	srv := r.server(t)
	if err := NewNtfy(srv.URL+"/sub/manga-x", srv.Client()).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	b := r.body
	actions, _ := b["actions"].([]any)
	if r.path != "/sub" || b["topic"] != "manga-x" || b["title"] != msg.Title || b["click"] != msg.Lines[0].URL ||
		b["message"] != "• Eleceed: cap. 424\n• Solo: caps. 9–10 (2 novos)" || len(actions) != 2 {
		t.Errorf("path %q body %v", r.path, b)
	}
}

func TestDiscord_PostsContentWithoutPreviewsOrMentions(t *testing.T) {
	r := &recorder{}
	srv := r.server(t)
	if err := NewDiscord(srv.URL+"/api/webhooks/1/x", srv.Client()).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	content, _ := r.body["content"].(string)
	if !strings.HasPrefix(content, "**LeftOff: 2 obras") || !strings.Contains(content, "<https://site.org/solo/10>") ||
		r.body["allowed_mentions"] == nil || r.body["username"] != "LeftOff" {
		t.Errorf("body %v", r.body)
	}
}

func TestTelegram_SendsPlainTextToTheChat(t *testing.T) {
	r := &recorder{}
	srv := r.server(t)
	if err := NewTelegram("123:SECRET", "-100", srv.URL, srv.Client()).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	text, _ := r.body["text"].(string)
	if r.path != "/bot123:SECRET/sendMessage" || r.body["chat_id"] != "-100" || r.body["disable_web_page_preview"] != true ||
		!strings.HasPrefix(text, msg.Title+"\n\n• Eleceed") {
		t.Errorf("path %q body %v", r.path, r.body)
	}
}

func TestErrors_NeverContainTheSecretURL(t *testing.T) {
	r := &recorder{status: http.StatusUnauthorized}
	srv := r.server(t)
	err := NewTelegram("123:SECRET", "1", srv.URL, srv.Client()).Send(context.Background(), msg)
	if err == nil || !strings.Contains(err.Error(), "telegram: HTTP 401") || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("err = %v", err)
	}
	srv.Close() // now the request itself fails
	err = NewTelegram("123:SECRET", "1", srv.URL, srv.Client()).Send(context.Background(), msg)
	if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), srv.URL) {
		t.Errorf("network err = %v", err)
	}
	if err := NewDiscord("://bad", nil).Send(context.Background(), msg); err == nil || strings.Contains(err.Error(), "bad") {
		t.Errorf("bad url err = %v", err)
	}
}

func TestRender_StopsBeforeTheLimit(t *testing.T) {
	m := Message{}
	for range 50 {
		m.Lines = append(m.Lines, Line{Text: strings.Repeat("x", 30), URL: "https://a.org/1"})
	}
	got := render(m, 300, func(u string) string { return u })
	if len(got) > 300 || !strings.HasSuffix(got, "… +45") || strings.Count(got, "\n") != 5 { // 5 lines of 50 bytes
		t.Errorf("render = %q (%d bytes)", got, len(got))
	}
	if render(Message{Lines: []Line{{Text: "a", URL: "u"}}}, 100, nil) != "• a" {
		t.Error("nil link must leave URLs out")
	}
	if got := render(Message{Text: "Hi", Lines: []Line{{Text: "a"}}}, 100, nil); got != "Hi\n• a" {
		t.Errorf("text before lines: %q", got)
	}
}

func TestCut(t *testing.T) {
	if cut("Eleceed", 40) != "Eleceed" || cut("abcdef", 4) != "abc…" {
		t.Error("cut")
	}
}
