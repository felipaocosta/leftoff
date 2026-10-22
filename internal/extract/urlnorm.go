package extract

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// trackingParams are query parameters removed during normalization.
var trackingParams = []string{"fbclid", "gclid", "mc_cid", "mc_eid", "ref", "igshid"}

// NormalizeURL parses raw, requires http/https, lowercases the host and
// removes the fragment and tracking parameters (utm_*, fbclid, ...).
func NormalizeURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("url %q must be absolute http(s): %w", raw, domain.ErrInvalidInput)
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	q := u.Query()
	for k := range q {
		if strings.HasPrefix(strings.ToLower(k), "utm_") {
			q.Del(k)
		}
	}
	for _, k := range trackingParams {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	return u, nil
}

// CanonicalKey identifies a series URL for duplicate detection (RF-04):
// scheme, "www." and a trailing slash do not matter.
func CanonicalKey(u *url.URL) string {
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	key := host + strings.TrimSuffix(u.EscapedPath(), "/")
	if u.RawQuery != "" {
		key += "?" + u.RawQuery
	}
	return key
}

// Domain returns the host of u without "www." (Series.Source.Domain).
func Domain(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// ResolveURL resolves href against base; it returns "" for empty, invalid or
// non-http(s) references such as "javascript:" or "#".
func ResolveURL(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	abs := base.ResolveReference(ref)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	return abs.String()
}
