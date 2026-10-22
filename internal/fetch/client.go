package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// DefaultUserAgent mimics a desktop browser; many scan sites reject Go's.
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0"

// Options configures a Client. Zero values select the SPEC §5.8 defaults.
type Options struct {
	UserAgent    string
	Timeout      time.Duration // per attempt; default 20s
	Retries      int           // extra attempts on transient errors; default 2, -1 disables
	RetryBackoff time.Duration // first backoff, doubled each retry; default 1s
	MinInterval  time.Duration // per host; default 2s (settings.rate_limit_ms)
	CacheTTL     time.Duration // default 10m, -1 disables
	MaxBodyBytes int64         // default 5 MB
	MaxRedirects int           // default 5
	Clock        domain.Clock  // default domain.SystemClock
	Logger       *slog.Logger  // default discards
	Transport    http.RoundTripper
	// Solvers are tried in order when a site answers with an anti-bot
	// challenge (RF-56); none means such pages fail with domain.ErrBlocked.
	Solvers []Solver
}

func (o Options) withDefaults() Options {
	def := func(v *time.Duration, d time.Duration) {
		if *v == 0 {
			*v = d
		}
	}
	def(&o.Timeout, 20*time.Second)
	def(&o.RetryBackoff, time.Second)
	def(&o.MinInterval, 2*time.Second)
	def(&o.CacheTTL, 10*time.Minute)
	if o.UserAgent == "" {
		o.UserAgent = DefaultUserAgent
	}
	if o.Retries == 0 {
		o.Retries = 2
	}
	if o.MaxBodyBytes <= 0 {
		o.MaxBodyBytes = 5 << 20
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = 5
	}
	if o.Clock == nil {
		o.Clock = domain.SystemClock{}
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	return o
}

// Client is the production Fetcher. It is safe for concurrent use.
type Client struct {
	opts       Options
	http       *http.Client
	gates      *hostGates
	cache      *responseCache
	clearances *clearances
}

// New builds a Client.
func New(opts Options) *Client {
	opts = opts.withDefaults()
	c := &Client{
		opts:       opts,
		gates:      newHostGates(opts.MinInterval, opts.Clock),
		cache:      newResponseCache(opts.CacheTTL, 256, opts.Clock),
		clearances: newClearances(),
	}
	c.http = &http.Client{
		Transport:     opts.Transport,
		Timeout:       opts.Timeout,
		CheckRedirect: c.checkRedirect,
	}
	return c
}

// Fetch implements Fetcher.
func (c *Client) Fetch(ctx context.Context, req *Request) (*Response, error) {
	if req.Method == "" {
		req = &Request{Method: http.MethodGet, URL: req.URL, Header: req.Header, Body: req.Body}
	}
	u, err := parseHTTPURL(req.URL)
	if err != nil {
		return nil, err
	}
	key := cacheKey(req)
	if resp, ok := c.cache.get(key); ok && !isFresh(ctx) {
		return resp, nil
	}
	resp, err := c.fetchWithRetries(ctx, req, u)
	if errors.Is(err, domain.ErrBlocked) && len(c.opts.Solvers) > 0 {
		resp, err = c.solve(ctx, req, u, err)
	}
	if err != nil {
		return nil, err
	}
	c.cache.put(key, resp)
	return resp, nil
}

// SolverNames lists the anti-bot solvers in the order they are tried.
func (c *Client) SolverNames() []string {
	names := make([]string, 0, len(c.opts.Solvers))
	for _, s := range c.opts.Solvers {
		names = append(names, s.Name())
	}
	return names
}

// solve runs the solvers in order while holding the host gate, so a site
// never gets parallel requests, and keeps the clearance of the first one
// that succeeds. If all fail, the original block error is returned with
// their reasons.
func (c *Client) solve(ctx context.Context, req *Request, u *url.URL, blocked error) (*Response, error) {
	release, err := c.gates.acquire(ctx, u.Host)
	if err != nil {
		return nil, err
	}
	defer release()
	errs := []error{blocked}
	for _, s := range c.opts.Solvers {
		c.opts.Logger.InfoContext(ctx, "solving anti-bot challenge", "host", u.Host, "solver", s.Name())
		sol, err := s.Solve(ctx, req)
		if err == nil {
			err = classify(sol.Response, req.URL)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.Name(), err))
			continue
		}
		if !sol.Clearance.empty() {
			c.clearances.set(u.Hostname(), sol.Clearance)
		}
		return sol.Response, nil
	}
	return nil, errors.Join(errs...)
}

func (c *Client) fetchWithRetries(ctx context.Context, req *Request, u *url.URL) (*Response, error) {
	backoff := c.opts.RetryBackoff
	for attempt := 0; ; attempt++ {
		resp, err := c.attempt(ctx, req, u)
		var perm *permanentError
		if errors.As(err, &perm) || ctx.Err() != nil {
			return nil, err
		}
		if !retryable(resp, err) || attempt >= c.opts.Retries {
			if err != nil {
				return nil, err
			}
			if err := classify(resp, req.URL); err != nil {
				return nil, err
			}
			return resp, nil
		}
		c.opts.Logger.DebugContext(ctx, "fetch retry", "host", u.Host, "attempt", attempt+1, "backoff", backoff)
		if err := sleep(ctx, backoff); err != nil {
			return nil, fmt.Errorf("fetch %s: %w", req.URL, err)
		}
		backoff *= 2
	}
}

// attempt performs one request through the host gate.
func (c *Client) attempt(ctx context.Context, req *Request, u *url.URL) (*Response, error) {
	release, err := c.gates.acquire(ctx, u.Host)
	if err != nil {
		return nil, err
	}
	defer release()

	hreq, err := http.NewRequestWithContext(ctx, req.Method, u.String(), bytes.NewReader(req.Body))
	if err != nil {
		return nil, &permanentError{fmt.Errorf("build request %s: %w", req.URL, err)}
	}
	hreq.Header.Set("User-Agent", c.opts.UserAgent)
	hreq.Header.Set("Accept", "text/html,application/xhtml+xml,application/json;q=0.9,*/*;q=0.8")
	hreq.Header.Set("Accept-Language", "en-US,en;q=0.8,pt-BR;q=0.6")
	for k, vs := range req.Header {
		hreq.Header[k] = vs
	}
	if cl, ok := c.clearances.get(u.Hostname()); ok {
		cl.apply(hreq)
	}

	hresp, err := c.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w: %w", req.URL, ErrUnreachable, err)
	}
	defer func() { _ = hresp.Body.Close() }() // body fully read below; close error is irrelevant

	body, err := io.ReadAll(io.LimitReader(hresp.Body, c.opts.MaxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w: %w", req.URL, ErrUnreachable, err)
	}
	if int64(len(body)) > c.opts.MaxBodyBytes {
		return nil, &permanentError{fmt.Errorf("fetch %s: %w (limit %d bytes)", req.URL, ErrTooLarge, c.opts.MaxBodyBytes)}
	}
	return &Response{URL: hresp.Request.URL, StatusCode: hresp.StatusCode, Header: hresp.Header, Body: body}, nil
}

func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > c.opts.MaxRedirects {
		return &permanentError{fmt.Errorf("stopped after %d redirects", c.opts.MaxRedirects)}
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return &permanentError{fmt.Errorf("redirect to %q: %w", req.URL.Scheme, domain.ErrInvalidInput)}
	}
	return nil
}

// parseHTTPURL accepts only absolute http/https URLs (RULES §11).
func parseHTTPURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("url %q must be absolute http(s): %w", raw, domain.ErrInvalidInput)
	}
	return u, nil
}

// permanentError marks failures that retrying cannot fix.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }
