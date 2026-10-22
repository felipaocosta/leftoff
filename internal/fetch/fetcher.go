package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Fetcher is what extractors use to download pages. Adapters never call
// net/http directly (RULES §6.5).
type Fetcher interface {
	Fetch(ctx context.Context, req *Request) (*Response, error)
}

// Request describes one HTTP request. Method defaults to GET.
type Request struct {
	Method string
	URL    string
	Header http.Header // extra headers; may be nil
	Body   []byte      // e.g. form data for an AJAX POST
}

// Response is a fully read HTTP response. Responses may be shared through
// the cache, so callers must not modify them.
type Response struct {
	URL        *url.URL // final URL after redirects
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Get is a shortcut for a plain GET request.
func Get(ctx context.Context, f Fetcher, rawURL string) (*Response, error) {
	return f.Fetch(ctx, &Request{Method: http.MethodGet, URL: rawURL})
}

// ErrTooLarge means the response body exceeded Options.MaxBodyBytes.
var ErrTooLarge = errors.New("response body too large")

// ErrUnreachable means the site could not be reached or kept redirecting:
// connection refused, unknown host, a dropped connection, a redirect loop.
var ErrUnreachable = errors.New("site unreachable")

// StatusError is returned for non-2xx responses that are not mapped to a
// domain error.
type StatusError struct {
	URL        string
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: unexpected status %d", e.URL, e.StatusCode)
}
