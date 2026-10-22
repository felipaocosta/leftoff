package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Driver is a browser tab that the host app controls: on Android, a
// WebView owned by the Kotlin shell (RF-56). Calls block until done and
// never come from the app's UI thread. Eval runs a JavaScript expression
// and returns its value as JSON (null for undefined).
type Driver interface {
	Load(rawURL string, timeout time.Duration) error // navigate; wait for the load event
	Eval(js string) (string, error)
	// Show puts the tab on screen so the user can pass a challenge, saying
	// which site asks; it reports false when nobody can see it (the app
	// is closed). Hide takes it off screen again.
	Show(host string) bool
	Hide()
	Cookies(rawURL string) string // "a=1; b=2", as the tab sends them
	UserAgent() string
}

// DrivenOptions tunes a Driven browser; zero values take the defaults.
type DrivenOptions struct {
	Interactive bool          // show the tab for challenges that need a person
	Timeout     time.Duration // how long a person may take; default 2 minutes
	poll        time.Duration // tests shorten the waits
	headless    time.Duration
	settleMax   time.Duration
	stableFor   time.Duration
}

// Driven solves challenges (fetch.Solver) and renders JavaScript pages
// (extract.Renderer) with a Driver, using the same page scripts as the
// desktop browser. One page at a time.
type Driven struct {
	d    Driver
	opts DrivenOptions
	mu   sync.Mutex
}

// NewDriven wraps a Driver.
func NewDriven(d Driver, opts DrivenOptions) *Driven {
	def := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}
	def(&opts.Timeout, 2*time.Minute)
	def(&opts.poll, pollEvery)
	def(&opts.headless, headlessWait)
	def(&opts.settleMax, 15*time.Second)
	def(&opts.stableFor, 1500*time.Millisecond)
	return &Driven{d: d, opts: opts}
}

// Name implements fetch.Solver.
func (*Driven) Name() string { return "webview" }

// loadTimeout bounds the load event of one page.
const loadTimeout = 30 * time.Second

// Solve implements fetch.Solver: the page through the tab, plus its
// cookies and User-Agent for the HTTP client.
func (b *Driven) Solve(ctx context.Context, req *fetch.Request) (*fetch.Solution, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.open(ctx, req, false)
}

// Render implements extract.Renderer: the page after its JavaScript ran,
// with the chapters tab opened and a client-side pager collected.
func (b *Driven) Render(ctx context.Context, rawURL string) (*fetch.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	sol, err := b.open(ctx, &fetch.Request{Method: http.MethodGet, URL: rawURL}, true)
	if err != nil {
		return nil, err
	}
	return sol.Response, nil
}

func (b *Driven) open(ctx context.Context, req *fetch.Request, settle bool) (*fetch.Solution, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("webview: %w", err)
	}
	target := req.URL
	if req.Method == http.MethodPost {
		target = u.Scheme + "://" + u.Host + "/" // pass the challenge on the site, then POST from it
	}
	if err := b.d.Load(target, loadTimeout); err != nil {
		return nil, fmt.Errorf("webview load %s: %w", target, err)
	}
	if err := b.passChallenge(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	if settle {
		b.settle(ctx)
	}
	return b.collect(ctx, req, u)
}

// passChallenge waits for a challenge to clear by itself and, when it does
// not, shows the tab so the user can pass it.
func (b *Driven) passChallenge(ctx context.Context, host string) error {
	err := b.waitCleared(ctx, b.opts.headless)
	if !errors.Is(err, errStillChallenged) {
		return err
	}
	if !b.opts.Interactive || !b.d.Show(host) {
		return fmt.Errorf("webview %s: %w: %w", host, domain.ErrBlocked, err)
	}
	defer b.d.Hide()
	if err := b.waitCleared(ctx, b.opts.Timeout); err != nil {
		return fmt.Errorf("webview %s: %w: %w", host, domain.ErrBlocked, err)
	}
	return nil
}

func (b *Driven) waitCleared(ctx context.Context, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		var challenged bool
		if err := b.eval(call(challengeJS), &challenged); err == nil && !challenged {
			return nil
		}
		if time.Now().After(deadline) {
			return errStillChallenged
		}
		if err := sleep(ctx, b.opts.poll); err != nil {
			return err
		}
	}
}

// settle waits until the number of links stops changing, opening a
// chapters tab and scrolling for lazy lists, then collects a pager.
func (b *Driven) settle(ctx context.Context) {
	deadline := time.Now().Add(b.opts.settleMax)
	last, stableSince := -1, time.Now()
	for time.Now().Before(deadline) && ctx.Err() == nil {
		_ = b.eval(call(openChaptersJS), nil) // best effort: pages without tabs just scroll
		var n int
		if b.eval(call(linkCountJS), &n) != nil {
			return
		}
		if n != last {
			last, stableSince = n, time.Now()
		} else if time.Since(stableSince) >= b.opts.stableFor {
			break
		}
		if sleep(ctx, b.opts.poll) != nil {
			return
		}
	}
	_ = b.evalAsync(ctx, call(collectPagesJS), nil, 45*time.Second) // pages without a pager return 0
}

// collect reads the page (or replays the POST from it) and the clearance.
func (b *Driven) collect(ctx context.Context, req *fetch.Request, u *url.URL) (*fetch.Solution, error) {
	status, body := http.StatusOK, ""
	if req.Method == http.MethodPost {
		ct := req.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/x-www-form-urlencoded; charset=UTF-8"
		}
		var res struct {
			Status int    `json:"status"`
			Text   string `json:"text"`
		}
		js := fmt.Sprintf("(%s)(%s, %s, %s)", postJS, quote(req.URL), quote(string(req.Body)), quote(ct))
		if err := b.evalAsync(ctx, js, &res, loadTimeout); err != nil {
			return nil, fmt.Errorf("webview post %s: %w", req.URL, err)
		}
		status, body = res.Status, res.Text
	} else if err := b.eval("document.documentElement.outerHTML", &body); err != nil {
		return nil, fmt.Errorf("webview read %s: %w", req.URL, err)
	}
	final := u
	var href string
	if b.eval("location.href", &href) == nil {
		if pu, err := url.Parse(href); err == nil && pu.Host != "" && req.Method != http.MethodPost {
			final = pu
		}
	}
	return &fetch.Solution{
		Response:  &fetch.Response{URL: final, StatusCode: status, Header: http.Header{}, Body: []byte(body)},
		Clearance: fetch.Clearance{Cookies: parseCookies(b.d.Cookies(req.URL), u.Hostname()), UserAgent: b.d.UserAgent()},
	}, nil
}

// eval runs js and decodes its JSON value into out (nil ignores it).
func (b *Driven) eval(js string, out any) error {
	raw, err := b.d.Eval(js)
	if err != nil {
		return fmt.Errorf("webview eval: %w", err)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("webview eval result %.40q: %w", raw, err)
	}
	return nil
}

// evalAsync runs a promise-returning expression: a WebView does not await
// promises, so the result is parked in the page and polled.
func (b *Driven) evalAsync(ctx context.Context, js string, out any, wait time.Duration) error {
	start := `(() => { window.__mtDone = false; Promise.resolve(` + js + `).then(
  v => { window.__mtVal = JSON.stringify(v === undefined ? null : v); window.__mtDone = true; },
  e => { window.__mtErr = String(e); window.__mtDone = true; }); return true; })()`
	if err := b.eval(start, nil); err != nil {
		return err
	}
	deadline := time.Now().Add(wait)
	for {
		var res struct {
			Done bool    `json:"done"`
			Val  *string `json:"val"`
			Err  *string `json:"err"`
		}
		if err := b.eval(`({done: window.__mtDone === true, val: window.__mtVal ?? null, err: window.__mtErr ?? null})`, &res); err != nil {
			return err
		}
		switch {
		case res.Done && res.Err != nil:
			return fmt.Errorf("webview script: %s", *res.Err)
		case res.Done:
			if out == nil || res.Val == nil {
				return nil
			}
			return json.Unmarshal([]byte(*res.Val), out)
		case time.Now().After(deadline):
			return fmt.Errorf("webview script: %w", context.DeadlineExceeded)
		}
		if err := sleep(ctx, b.opts.poll); err != nil {
			return err
		}
	}
}

// call turns a function source into a call expression.
func call(fn string) string { return "(" + fn + ")()" }

func quote(s string) string {
	b, _ := json.Marshal(s) // a string always encodes
	return string(b)
}

// parseCookies reads a Cookie header value for host.
func parseCookies(header, host string) []*http.Cookie {
	var out []*http.Cookie
	for part := range strings.SplitSeq(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && name != "" {
			out = append(out, &http.Cookie{Name: name, Value: value, Domain: host, Path: "/"})
		}
	}
	return out
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
