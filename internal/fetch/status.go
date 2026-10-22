package fetch

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// challengeMarkers are body fragments of anti-bot interstitial pages.
var challengeMarkers = [][]byte{
	[]byte("challenge-platform"),
	[]byte("<title>Just a moment...</title>"),
	[]byte("cf-browser-verification"),
}

// classify turns a completed response into an error, or nil for 2xx.
func classify(resp *Response, rawURL string) error {
	code := resp.StatusCode
	switch {
	case code >= 200 && code < 300:
		return nil
	case code == http.StatusNotFound || code == http.StatusGone:
		return fmt.Errorf("fetch %s: status %d: %w", rawURL, code, domain.ErrNotFound)
	case isChallenge(resp):
		return fmt.Errorf("fetch %s: anti-bot challenge (status %d): %w", rawURL, code, domain.ErrBlocked)
	default:
		return &StatusError{URL: rawURL, StatusCode: code}
	}
}

func isChallenge(resp *Response) bool {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusServiceUnavailable {
		return false
	}
	if strings.EqualFold(resp.Header.Get("Cf-Mitigated"), "challenge") {
		return true
	}
	for _, m := range challengeMarkers {
		if bytes.Contains(resp.Body, m) {
			return true
		}
	}
	return false
}

// retryable reports whether a failed attempt may succeed if repeated.
func retryable(resp *Response, err error) bool {
	if err != nil {
		return true // network errors and timeouts
	}
	return resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 && !isChallenge(resp)
}
