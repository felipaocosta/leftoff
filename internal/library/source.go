package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// ErrDifferentSeries means a new source URL does not look like the same
// work (no shared title nor chapter); the caller may insist with force.
var ErrDifferentSeries = errors.New("the new address looks like a different series")

// ChangeSource moves a series to a new address (RF-08), e.g. when its site
// changed domain without redirecting. The new page must look like the same
// work unless force is set; chapters are merged as in a refresh, so reading
// state is kept, and the old address becomes a mirror.
func (s *Service) ChangeSource(ctx context.Context, id, rawURL string, force bool) (domain.Series, error) {
	current, err := s.Get(id)
	if err != nil {
		return domain.Series{}, err
	}
	u, err := extract.NormalizeURL(rawURL)
	if err != nil {
		return domain.Series{}, err
	}
	res, err := s.extractor.Extract(ctx, u.String())
	if err != nil {
		return domain.Series{}, fmt.Errorf("change source of %s: %w", id, err)
	}
	if !force && !sameWork(current, res) {
		return domain.Series{}, fmt.Errorf("change source of %s to %s: %w", id, res.Series.URL, ErrDifferentSeries)
	}
	err = s.store.Update(ctx, func(lib *storage.Library) error {
		i, err := find(lib, id)
		if err != nil {
			return err
		}
		if err := checkDuplicateExcept(lib, id, u, mustParse(res.Series.URL)); err != nil {
			return err
		}
		sr := &lib.Series[i]
		merged, err := mergeChapters(sr.Chapters, res.Chapters, s.clock.Now())
		if err != nil {
			return err
		}
		sr.Chapters = merged.Chapters
		migrateSource(sr, res.Series.URL, res.Method)
		now := s.clock.Now()
		sr.LastCheckedAt, sr.CheckError = &now, nil
		return nil
	})
	if err != nil {
		return domain.Series{}, err
	}
	return s.Get(id)
}

// migrateSource points the series at newURL when it is a different
// address, keeping the old one as a mirror. It reports whether it moved.
func migrateSource(sr *domain.Series, newURL, method string) bool {
	nu, err := url.Parse(newURL)
	if err != nil || newURL == "" {
		return false
	}
	old, err := url.Parse(sr.Source.URL)
	if err == nil && extract.CanonicalKey(old) == extract.CanonicalKey(nu) {
		return false
	}
	mirrors := []string{}
	for _, m := range append([]string{sr.Source.URL}, sr.Source.Mirrors...) {
		mu, err := url.Parse(m)
		if m == "" || err != nil || extract.CanonicalKey(mu) == extract.CanonicalKey(nu) || containsKey(mirrors, mu) {
			continue
		}
		mirrors = append(mirrors, m)
	}
	sr.Source.Mirrors = mirrors
	sr.Source.URL, sr.Source.Domain = nu.String(), extract.Domain(nu)
	if method != "" {
		sr.Source.Method = method
	}
	return true
}

func containsKey(list []string, u *url.URL) bool {
	for _, raw := range list {
		if v, err := url.Parse(raw); err == nil && extract.CanonicalKey(v) == extract.CanonicalKey(u) {
			return true
		}
	}
	return false
}

// sameWork reports whether an extraction looks like the stored series:
// same normalized title or alternative title, same URL slug, or at least
// one chapter at the same URL path (sites that only changed domain).
func sameWork(sr domain.Series, res *extract.Result) bool {
	names := map[string]bool{}
	for _, t := range append([]string{sr.Title}, sr.AltTitles...) {
		names[domain.NormalizeLabel(t)] = true
	}
	for _, t := range append([]string{res.Series.Title}, res.Series.AltTitles...) {
		if n := domain.NormalizeLabel(t); n != "" && names[n] {
			return true
		}
	}
	if slug(sr.Source.URL) != "" && slug(sr.Source.URL) == slug(res.Series.URL) {
		return true
	}
	paths := map[string]bool{}
	for _, c := range sr.Chapters {
		if p := urlPath(c.URL); p != "" {
			paths[p] = true
		}
	}
	for _, c := range res.Chapters {
		if paths[urlPath(c.URL)] {
			return true
		}
	}
	return false
}

// slug is the last path segment of a series URL ("survival-supremacy").
func slug(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	s := path.Base(strings.TrimSuffix(u.Path, "/"))
	if s == "." || s == "/" {
		return ""
	}
	return strings.ToLower(s)
}

// checkDuplicateExcept is checkDuplicate ignoring the series being moved.
func checkDuplicateExcept(lib *storage.Library, id string, urls ...*url.URL) error {
	others := &storage.Library{}
	for _, sr := range lib.Series {
		if sr.ID != id {
			others.Series = append(others.Series, sr)
		}
	}
	return checkDuplicate(others, urls...)
}

func mustParse(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		return &url.URL{}
	}
	return u
}
