package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// Static is an in-memory Fetcher that serves fixed bodies by URL. It backs
// offline tests against saved fixtures (RULES §1.4) and lets an already
// downloaded page be re-extracted without hitting the site again.
// It is not safe for concurrent use.
type Static struct {
	// Pages maps "URL" (GET) or "POST URL" to a response body.
	Pages map[string][]byte
	// Requests records every request key received, in order.
	Requests []string
}

// Fetch implements Fetcher. Unknown URLs yield domain.ErrNotFound.
func (s *Static) Fetch(_ context.Context, req *Request) (*Response, error) {
	key := req.URL
	if req.Method != "" && req.Method != http.MethodGet {
		key = req.Method + " " + req.URL
	}
	s.Requests = append(s.Requests, key)
	body, ok := s.Pages[key]
	if !ok {
		return nil, fmt.Errorf("static fetch %s: %w", key, domain.ErrNotFound)
	}
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("static fetch %s: %w", req.URL, domain.ErrInvalidInput)
	}
	return &Response{URL: u, StatusCode: http.StatusOK, Header: http.Header{}, Body: body}, nil
}
