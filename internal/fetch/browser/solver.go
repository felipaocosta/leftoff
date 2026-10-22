//go:build rod

package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"

	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Solver drives a local browser. One solve runs at a time; the browser is
// started for each solve and closed afterwards.
type Solver struct {
	opts Options
	mu   sync.Mutex
	// interactiveOnly remembers hosts where headless never passes, so later
	// pages go straight to the window instead of waiting headlessWait again.
	interactiveOnly map[string]bool
}

// New returns the browser solver.
func New(opts Options) *Solver {
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	return &Solver{opts: opts, interactiveOnly: map[string]bool{}}
}

// Available reports whether this build can drive a browser.
func Available() bool { return true }

// Name implements fetch.Solver.
func (*Solver) Name() string { return "browser" }

// Solve implements fetch.Solver: headless first, then (if allowed) a
// visible window the user can interact with.
func (s *Solver) Solve(ctx context.Context, req *fetch.Request) (*fetch.Solution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bin, err := s.find(ctx)
	if err != nil {
		return nil, err
	}
	return s.solve(ctx, bin, req, false)
}

// solve tries headless first (unless the host is known to need a person)
// and then, if allowed, a visible window.
func (s *Solver) solve(ctx context.Context, bin binary, req *fetch.Request, settle bool) (*fetch.Solution, error) {
	host := hostOf(req.URL)
	if !s.interactiveOnly[host] || !s.opts.Interactive {
		sol, err := s.run(ctx, bin, req, runOptions{headless: true, wait: headlessWait, settle: settle})
		if err == nil || !s.opts.Interactive || !errors.Is(err, errStillChallenged) {
			return sol, err
		}
		s.interactiveOnly[host] = true
	}
	return s.run(ctx, bin, req, runOptions{wait: s.opts.Timeout, settle: settle})
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// Render implements extract.Renderer: it returns the page after its
// JavaScript ran, opening the chapters tab when there is one. It runs
// without a window unless a challenge needs the user.
func (s *Solver) Render(ctx context.Context, rawURL string) (*fetch.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bin, err := s.find(ctx)
	if err != nil {
		return nil, err
	}
	sol, err := s.solve(ctx, bin, &fetch.Request{Method: http.MethodGet, URL: rawURL}, true)
	if err != nil {
		return nil, err
	}
	return sol.Response, nil
}

// runOptions selects how one browser session behaves.
type runOptions struct {
	headless bool
	wait     time.Duration // how long a challenge may take
	settle   bool          // wait for JavaScript content (Render)
}

func (s *Solver) find(ctx context.Context) (binary, error) {
	profile := s.opts.ProfileDir
	if s.opts.Bin != "" {
		return binary{path: s.opts.Bin, profile: profile}, nil
	}
	if path, ok := launcher.LookPath(); ok {
		return binary{path: path, profile: profile}, nil
	}
	b, err := findFlatpak(ctx, filepath.Join(filepath.Dir(profile), "browser-wrappers"))
	if err != nil {
		return binary{}, fmt.Errorf("%w: %w", fetch.ErrNoBrowser, err)
	}
	return b, nil
}

func (s *Solver) run(ctx context.Context, bin binary, req *fetch.Request, ro runOptions) (*fetch.Solution, error) {
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("browser: %w", err)
	}
	target := req.URL
	if req.Method == http.MethodPost {
		target = u.Scheme + "://" + u.Host + "/" // pass the challenge on the site, then POST from it
	}
	l := launcher.New().Context(ctx).Bin(bin.path).Leakless(false).Headless(ro.headless).UserDataDir(bin.profile)
	l.Delete(flags.Flag("enable-automation"))
	if ro.headless {
		l.Set(flags.Headless, "new")
	} else {
		// The window opens the page itself (no DevTools session touches it
		// while the challenge runs: Cloudflare detects one and loops) and
		// looks like a normal browser: only the flags it really needs.
		minimalFlags(l)
		l.Set(flags.Arguments, target)
	}
	// Kill ends the launcher's process; a Flatpak browser keeps running
	// inside its sandbox, so whatever still uses our profile ends too.
	defer func() { l.Kill(); reapProfile(bin.profile) }()
	wsURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch browser %s: %w", bin.path, err)
	}
	b := rod.New().Context(ctx).ControlURL(wsURL)
	var page *rod.Page
	if ro.headless {
		page, err = openHeadless(ctx, b, target, ro.wait)
	} else {
		page, err = openInteractive(ctx, b, wsURL, u.Hostname(), ro.wait)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = b.Close() }() // the launcher kill above is the real cleanup
	if ro.settle {
		settle(ctx, page)
	}
	return collect(page, req, u)
}

// keptFlags are the only launcher flags a visible window gets. go-rod's
// defaults (disable-site-isolation-trials, disable-popup-blocking, …) are
// automation fingerprints that strict challenges such as comix.to reject.
var keptFlags = map[flags.Flag]bool{
	flags.Bin: true, flags.Leakless: true, flags.UserDataDir: true,
	flags.RemoteDebuggingPort: true, flags.Arguments: true, "no-first-run": true,
}

func minimalFlags(l *launcher.Launcher) {
	for name := range l.Flags {
		if !keptFlags[name] {
			l.Delete(name)
		}
	}
}

// openHeadless loads target in a new headless tab with the headless marker
// hidden and waits for any challenge to clear.
func openHeadless(ctx context.Context, b *rod.Browser, target string, wait time.Duration) (*rod.Page, error) {
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("connect to browser: %w", err)
	}
	page, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, fmt.Errorf("open page: %w", err)
	}
	if err := hideHeadless(b, page); err != nil {
		return nil, err
	}
	if err := page.Navigate(target); err != nil {
		return nil, fmt.Errorf("open %s: %w", target, err)
	}
	if err := waitCleared(ctx, page, wait); err != nil {
		return nil, err
	}
	return page, nil
}

// openInteractive waits, through the DevTools HTTP list only, until the tab
// the window opened shows a real page, and only then attaches to it.
func openInteractive(ctx context.Context, b *rod.Browser, wsURL, host string, wait time.Duration) (*rod.Page, error) {
	id, err := waitTabCleared(ctx, wsURL, host, wait)
	if err != nil {
		return nil, err
	}
	if err := b.Connect(); err != nil {
		return nil, fmt.Errorf("connect to browser: %w", err)
	}
	page, err := b.PageFromTarget(proto.TargetTargetID(id))
	if err != nil {
		return nil, fmt.Errorf("attach to page: %w", err)
	}
	return page, nil
}

// challengeTitleRe matches challenge page titles (en/pt/es).
var challengeTitleRe = regexp.MustCompile(`(?i)just a moment|um momento|un momento|attention required`)

// waitTabCleared polls <devtools>/json/list until a page on host has kept
// a real (non-challenge) title for two polls in a row.
func waitTabCleared(ctx context.Context, wsURL, host string, wait time.Duration) (string, error) {
	ws, err := url.Parse(wsURL)
	if err != nil {
		return "", fmt.Errorf("devtools url: %w", err)
	}
	list := "http://" + ws.Host + "/json/list"
	deadline := time.Now().Add(wait)
	lastGood := ""
	for time.Now().Before(deadline) {
		id := clearedTab(ctx, list, host)
		if id != "" && id == lastGood {
			return id, nil
		}
		lastGood = id
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return "", errStillChallenged
}

// clearedTab returns the id of a page on host whose title is a real page
// title, or "".
func clearedTab(ctx context.Context, list, host string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, list, nil)
	if err != nil {
		return ""
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }() // read-only local endpoint
	var tabs []struct{ ID, Type, Title, URL string }
	if json.NewDecoder(resp.Body).Decode(&tabs) != nil {
		return ""
	}
	for _, t := range tabs {
		tu, err := url.Parse(t.URL)
		title := strings.TrimSpace(t.Title)
		if t.Type != "page" || err != nil || !strings.HasSuffix(tu.Hostname(), strings.TrimPrefix(host, "www.")) {
			continue
		}
		if title != "" && !challengeTitleRe.MatchString(title) && !strings.EqualFold(title, tu.Hostname()) && title != t.URL {
			return t.ID
		}
	}
	return ""
}

// settle opens the chapters tab and waits until the number of links stops
// growing (JavaScript lists), up to settleMax. It is best effort.
func settle(ctx context.Context, page *rod.Page) {
	const settleMax, stableFor = 15 * time.Second, 1500 * time.Millisecond
	deadline := time.Now().Add(settleMax)
	last, stableSince := -1, time.Now()
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}
		_, _ = page.Eval(openChaptersJS) // best effort: pages without tabs just scroll
		res, err := page.Eval(linkCountJS)
		if err != nil {
			return
		}
		if n := res.Value.Int(); n != last {
			last, stableSince = n, time.Now()
		} else if time.Since(stableSince) >= stableFor {
			break // stable: go on to collect paginated lists
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollEvery):
		}
	}
	collectPages(page)
}

// collectPages copies the links of every page of a client-side pager into
// the DOM (best effort, at most collectMax).
func collectPages(page *rod.Page) {
	const collectMax = 45 * time.Second
	_, _ = page.Timeout(collectMax).Eval(collectPagesJS) // pages without a pager return 0
}

// hideHeadless replaces "HeadlessChrome" in the User-Agent with "Chrome":
// sites treat the headless marker as a bot signal.
func hideHeadless(b *rod.Browser, page *rod.Page) error {
	v, err := proto.BrowserGetVersion{}.Call(b)
	if err != nil {
		return fmt.Errorf("browser version: %w", err)
	}
	ua := strings.Replace(v.UserAgent, "HeadlessChrome", "Chrome", 1)
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: ua}); err != nil {
		return fmt.Errorf("set user agent: %w", err)
	}
	return nil
}

// waitCleared polls the page until it is no longer a challenge.
func waitCleared(ctx context.Context, page *rod.Page, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		res, err := page.Eval(challengeJS)
		if err == nil && !res.Value.Bool() {
			_ = page.WaitIdle(2 * time.Second) // best effort: let late scripts settle
			return nil
		}
		if time.Now().After(deadline) {
			return errStillChallenged
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
	}
}

// collect reads the page (or replays the POST) and the clearance.
func collect(page *rod.Page, req *fetch.Request, u *url.URL) (*fetch.Solution, error) {
	status, body, err := readBody(page, req)
	if err != nil {
		return nil, err
	}
	ua, err := page.Eval(`() => navigator.userAgent`)
	if err != nil {
		return nil, fmt.Errorf("browser user agent: %w", err)
	}
	cookies, err := page.Cookies([]string{req.URL})
	if err != nil {
		return nil, fmt.Errorf("browser cookies: %w", err)
	}
	cl := fetch.Clearance{UserAgent: strings.Replace(ua.Value.Str(), "HeadlessChrome", "Chrome", 1)}
	for _, c := range cookies {
		cl.Cookies = append(cl.Cookies, &http.Cookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path})
	}
	final := u
	if info, err := page.Info(); err == nil && info.URL != "" {
		if pu, err := url.Parse(info.URL); err == nil {
			final = pu
		}
	}
	return &fetch.Solution{
		Response:  &fetch.Response{URL: final, StatusCode: status, Header: http.Header{}, Body: []byte(body)},
		Clearance: cl,
	}, nil
}

// readBody returns the page HTML, or replays a POST from the page so the
// request carries the page's cookies.
func readBody(page *rod.Page, req *fetch.Request) (int, string, error) {
	if req.Method != http.MethodPost {
		html, err := page.HTML()
		if err != nil {
			return 0, "", fmt.Errorf("browser read %s: %w", req.URL, err)
		}
		return http.StatusOK, html, nil
	}
	ct := req.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/x-www-form-urlencoded; charset=UTF-8"
	}
	res, err := page.Eval(postJS, req.URL, string(req.Body), ct)
	if err != nil {
		return 0, "", fmt.Errorf("browser post %s: %w", req.URL, err)
	}
	return res.Value.Get("status").Int(), res.Value.Get("text").Str(), nil
}
