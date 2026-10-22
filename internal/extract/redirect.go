package extract

import (
	"context"
	"net/url"
	"strings"
	"sync"

	"github.com/felipaocosta/leftoff/internal/fetch"
)

// redirectRecorder wraps a Fetcher and remembers when a request to the
// series host ended on another host, i.e. the site moved (RF-08). Adapters
// report the URL they were given; the pipeline uses this to report the new
// address instead.
type redirectRecorder struct {
	inner fetch.Fetcher
	host  string // host of the series URL

	mu    sync.Mutex
	moved *url.URL // final URL of the first cross-host redirect
}

func newRedirectRecorder(f fetch.Fetcher, u *url.URL) *redirectRecorder {
	return &redirectRecorder{inner: f, host: strings.ToLower(u.Hostname())}
}

// Fetch implements fetch.Fetcher.
func (r *redirectRecorder) Fetch(ctx context.Context, req *fetch.Request) (*fetch.Response, error) {
	resp, err := r.inner.Fetch(ctx, req)
	if err != nil || resp.URL == nil {
		return resp, err
	}
	reqURL, perr := url.Parse(req.URL)
	if perr == nil && strings.EqualFold(reqURL.Hostname(), r.host) && !strings.EqualFold(resp.URL.Hostname(), r.host) {
		r.mu.Lock()
		if r.moved == nil {
			r.moved = resp.URL
		}
		r.mu.Unlock()
	}
	return resp, nil
}

// movedURL returns seriesURL on the host the site redirected to, or "" if
// it did not move.
func (r *redirectRecorder) movedURL(seriesURL string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.moved == nil {
		return ""
	}
	u, err := url.Parse(seriesURL)
	if err != nil {
		return ""
	}
	u.Scheme, u.Host = r.moved.Scheme, r.moved.Host
	return u.String()
}
