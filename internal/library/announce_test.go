package library

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// inbox is a fake ntfy and Telegram server.
type inbox struct {
	mu       sync.Mutex
	messages []map[string]any
	down     bool
}

func (in *inbox) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		in.mu.Lock()
		defer in.mu.Unlock()
		if in.down {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["path"] = r.URL.Path
		in.messages = append(in.messages, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newAnnouncer(t *testing.T, e *env, n NotificationSettings) (*Announcer, *inbox) {
	t.Helper()
	in := &inbox{}
	srv := in.server(t)
	if n.NtfyURL == "use-fake" {
		n.NtfyURL = srv.URL + "/manga-x"
	}
	settings := NewSettings(e.store, NotifyEnv{})
	if _, err := settings.Update(context.Background(), SettingsUpdate{Notifications: &n}); err != nil {
		t.Fatal(err)
	}
	a := NewAnnouncer(e.svc, settings, srv.Client(), nil)
	a.telegramAPI = srv.URL
	return a, in
}

func TestAnnounce_GroupsNewChaptersAndFilters(t *testing.T) {
	e := newEnv(t)
	alpha, beta, gamma := seedLibrary(t, e) // beta is on hold
	a, in := newAnnouncer(t, e, NotificationSettings{NtfyURL: "use-fake", OnlyReading: true})
	added := func(id string, chs ...domain.Chapter) RefreshReport { return RefreshReport{SeriesID: id, Added: chs} }

	a.Announce(context.Background(), []RefreshReport{
		added(alpha.ID, domain.Chapter{Number: num(2), URL: "https://site.org/a/2"}),
		added(beta.ID, domain.Chapter{Number: num(4), URL: "https://site.org/b/4"}),
		added(gamma.ID, domain.Chapter{Number: num(4), URL: "https://site.org/g/4"}, domain.Chapter{Number: num(5), URL: "https://site.org/g/5"}),
		{SeriesID: "removed", Added: []domain.Chapter{{Label: "x"}}},
	})
	if len(in.messages) != 1 {
		t.Fatalf("messages = %v", in.messages)
	}
	m := in.messages[0]
	if m["title"] != "LeftOff: 2 obras com capítulos novos" || m["click"] != "https://site.org/a/2" ||
		m["message"] != "• Alpha: cap. 2\n• Gamma: caps. 4–5 (2 novos)" {
		t.Errorf("message = %v", m)
	}

	a.Announce(context.Background(), nil)
	if len(in.messages) != 1 {
		t.Error("nothing new and nothing pending must send nothing")
	}
}

func TestAnnounce_ResendsWhatFailed(t *testing.T) {
	e := newEnv(t)
	alpha, _, gamma := seedLibrary(t, e)
	a, in := newAnnouncer(t, e, NotificationSettings{NtfyURL: "use-fake", Language: LangEn})
	in.down = true
	a.Announce(context.Background(), []RefreshReport{{SeriesID: alpha.ID, Added: []domain.Chapter{{Number: num(10.5), URL: "u1"}}}})
	in.down = false
	a.Announce(context.Background(), []RefreshReport{{SeriesID: gamma.ID, Added: []domain.Chapter{{Label: "Extra", URL: "u2"}}}})
	if len(in.messages) != 1 || in.messages[0]["message"] != "• Alpha: ch. 10.5\n• Gamma: ch. Extra" {
		t.Fatalf("messages = %v", in.messages)
	}
	a.Announce(context.Background(), nil)
	if len(in.messages) != 1 {
		t.Error("delivered lines must not be sent again")
	}
}

func TestAnnouncer_Test(t *testing.T) {
	e := newEnv(t)
	a, _ := newAnnouncer(t, e, NotificationSettings{})
	if _, err := a.Test(context.Background()); !errors.Is(err, ErrNoChannels) || !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("no channels: %v", err)
	}

	a, in := newAnnouncer(t, newEnv(t), NotificationSettings{
		NtfyURL: "use-fake", Telegram: TelegramSettings{Token: tgToken, ChatID: "42"},
	})
	results, err := a.Test(context.Background())
	if err != nil || len(results) != 2 || !results[0].OK || !results[1].OK {
		t.Fatalf("results = %+v, %v", results, err)
	}
	if tg := in.messages[1]; tg["path"] != "/bot"+tgToken+"/sendMessage" || !strings.Contains(tg["text"].(string), "teste") {
		t.Errorf("telegram = %v", tg)
	}
	in.down = true
	results, _ = a.Test(context.Background())
	if results[0].OK || !strings.Contains(results[0].Error, "HTTP 502") || strings.Contains(results[1].Error, tgToken) {
		t.Errorf("failure results = %+v", results)
	}
}

func TestLine(t *testing.T) {
	tests := []struct {
		lang  string
		added []domain.Chapter
		want  string
		url   string
	}{
		{LangPtBR, []domain.Chapter{{Number: num(40.5), URL: "a"}}, "S: cap. 40,5", "a"},
		{LangPtBR, []domain.Chapter{{Label: "Afterword", URL: "b"}}, "S: cap. Afterword", "b"},
		{LangPtBR, []domain.Chapter{{Number: num(9), URL: "9"}, {Number: num(11), URL: "11"}, {Number: num(10), URL: "10"}}, "S: caps. 9–11 (3 novos)", "11"},
		{LangEn, []domain.Chapter{{Number: num(3), URL: "x"}, {Number: num(3), URL: "y"}}, "S: 2 new chapters", "x"},
		{LangEn, []domain.Chapter{{Label: "A", URL: "x"}, {Label: "B", URL: "y"}}, "S: 2 new chapters", "x"},
		{"xx", []domain.Chapter{{Number: num(1), URL: "z"}}, "S: cap. 1", "z"},
	}
	for _, tt := range tests {
		if got := line(tt.lang, "S", tt.added); got.Text != tt.want || got.URL != tt.url {
			t.Errorf("line(%s, %v) = %+v, want %q %q", tt.lang, tt.added, got, tt.want, tt.url)
		}
	}
	if m := message(LangEn, nil); m.Title != "LeftOff: new chapter" {
		t.Errorf("title = %q", m.Title)
	}
}
