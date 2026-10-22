package browser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// fakeTab plays a page: a challenge that clears after some polls (or only
// once shown, as one that needs a click), then a list that grows a little.
type fakeTab struct {
	mu          sync.Mutex
	html        string
	challenge   int  // polls before the challenge clears; -1: only after Show
	canShow     bool // false: the app is closed
	shown, hid  int
	links       []int // link counts seen by successive polls
	loaded      []string
	asyncResult string // JSON value of a promise (collect pages, POST)
}

func (f *fakeTab) Load(u string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loaded = append(f.loaded, u)
	return nil
}

func (f *fakeTab) Eval(js string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case strings.Contains(js, "_cf_chl_opt"):
		if f.challenge == -1 {
			return jsonOf(f.shown == 0), nil
		}
		f.challenge--
		return jsonOf(f.challenge >= 0), nil
	case strings.Contains(js, "__mtTabClicked"):
		return "false", nil
	case strings.Contains(js, "querySelectorAll('a[href]').length"):
		n := f.links[0]
		if len(f.links) > 1 {
			f.links = f.links[1:]
		}
		return jsonOf(n), nil
	case strings.Contains(js, "window.__mtDone = false"):
		return "true", nil
	case strings.Contains(js, "done: window.__mtDone"):
		return jsonOf(map[string]any{"done": true, "val": f.asyncResult, "err": nil}), nil
	case js == "document.documentElement.outerHTML":
		return jsonOf(f.html), nil
	case js == "location.href":
		return jsonOf("https://site.org/manga/x/"), nil
	}
	return "null", nil
}

func (f *fakeTab) Show(string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.canShow {
		f.shown++
	}
	return f.canShow
}

func (f *fakeTab) Hide()                 { f.mu.Lock(); f.hid++; f.mu.Unlock() }
func (f *fakeTab) Cookies(string) string { return "cf_clearance=abc; session=1" }
func (f *fakeTab) UserAgent() string     { return "Mozilla/5.0 (Linux; Android 15) WebView" }

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func fast(interactive bool) DrivenOptions {
	ms := time.Millisecond
	return DrivenOptions{Interactive: interactive, Timeout: 200 * ms, poll: ms, headless: 20 * ms, settleMax: 100 * ms, stableFor: 5 * ms}
}

func TestDriven_RenderWaitsForTheChallengeAndTheList(t *testing.T) {
	tab := &fakeTab{html: "<h1>X</h1><a href=c1>Chapter 1</a>", challenge: 2, links: []int{3, 40, 120, 120}, asyncResult: "0"}
	resp, err := NewDriven(tab, fast(true)).Render(context.Background(), "https://site.org/manga/x/")
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != tab.html || resp.URL.String() != "https://site.org/manga/x/" || tab.shown != 0 {
		t.Errorf("resp = %q %s, shown %d", resp.Body, resp.URL, tab.shown)
	}
	if len(tab.links) != 1 {
		t.Errorf("the list was not followed until stable: %v left", tab.links)
	}
}

func TestDriven_ShowsTheTabForAChallengeThatNeedsTheUser(t *testing.T) {
	tab := &fakeTab{html: "<p>ok</p>", challenge: -1, canShow: true, links: []int{1}}
	sol, err := NewDriven(tab, fast(true)).Solve(context.Background(), &fetch.Request{Method: http.MethodGet, URL: "https://site.org/x"})
	if err != nil {
		t.Fatal(err)
	}
	if tab.shown != 1 || tab.hid != 1 {
		t.Errorf("shown %d, hidden %d", tab.shown, tab.hid)
	}
	cl := sol.Clearance
	if cl.UserAgent == "" || len(cl.Cookies) != 2 || cl.Cookies[0].Name != "cf_clearance" || cl.Cookies[0].Domain != "site.org" {
		t.Errorf("clearance = %+v", cl)
	}
}

func TestDriven_BlockedWhenNobodyCanPassTheChallenge(t *testing.T) {
	for name, tc := range map[string]struct {
		interactive, canShow bool
	}{"app closed": {true, false}, "not interactive": {false, true}} {
		t.Run(name, func(t *testing.T) {
			tab := &fakeTab{challenge: -1, canShow: tc.canShow, links: []int{1}}
			_, err := NewDriven(tab, fast(tc.interactive)).Render(context.Background(), "https://site.org/x")
			if !errors.Is(err, domain.ErrBlocked) || tab.shown != 0 {
				t.Errorf("err = %v, shown %d", err, tab.shown)
			}
		})
	}
}

func TestDriven_ReplaysAPostFromThePage(t *testing.T) {
	tab := &fakeTab{challenge: 0, links: []int{1}, asyncResult: jsonOf(map[string]any{"status": 200, "text": "<li>Chapter 2</li>"})}
	req := &fetch.Request{Method: http.MethodPost, URL: "https://site.org/manga/x/ajax/chapters/", Body: []byte("a=1")}
	sol, err := NewDriven(tab, fast(true)).Solve(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if string(sol.Response.Body) != "<li>Chapter 2</li>" || tab.loaded[0] != "https://site.org/" {
		t.Errorf("body %q, loaded %v", sol.Response.Body, tab.loaded)
	}
}
