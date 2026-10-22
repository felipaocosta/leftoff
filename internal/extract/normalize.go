package extract

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// ErrIncomplete means a required field (the title) was not found on the
// page, so the extraction is rejected instead of inventing a value.
var ErrIncomplete = errors.New("required field not found")

// normalizeResult applies SPEC §5.6 and §5.7 to a raw result in place:
// absolute URLs, clean text, normalized status, labels for every chapter and
// duplicate chapters folded into alt_urls. A result without title is an
// error (RULES §6.7).
func normalizeResult(r *Result, base *url.URL) error {
	s := &r.Series
	s.Title = CleanText(s.Title)
	if s.Title == "" {
		return fmt.Errorf("%s: series title: %w", r.Method, ErrIncomplete)
	}
	if s.URL == "" {
		s.URL = base.String()
	}
	s.CoverURL = ResolveURL(base, s.CoverURL)
	s.StatusRaw = CleanText(s.StatusRaw)
	if s.Status == "" {
		s.Status = domain.NormalizeStatus(s.StatusRaw)
	}
	if s.Type == "" {
		s.Type = domain.TypeOther
	}
	r.Chapters = normalizeChapters(r.Chapters, base)
	r.Confidence = Confidence(r.Series, r.Chapters)
	return nil
}

// normalizeChapters keeps source order, drops chapters without label and
// number and release dates that cannot be real, and keeps the first of each stable key, recording the URLs of the
// others in AltURLs.
func normalizeChapters(in []domain.ChapterInfo, base *url.URL) []domain.ChapterInfo {
	out := make([]domain.ChapterInfo, 0, len(in))
	index := make(map[string]int, len(in))
	for _, c := range in {
		c.Label = CleanText(c.Label)
		c.URL = ResolveURL(base, c.URL)
		if !c.ReleasedAt.IsZero() && !c.ReleasedAt.Plausible() {
			c.ReleasedAt = domain.Date{} // a misread date (a huge timestamp) is no date
		}
		if c.Label == "" && c.Number == nil {
			continue
		}
		if c.Label == "" {
			c.Label = fmt.Sprintf("Chapter %s", domain.ChapterKey(c.Number, ""))
		}
		key := c.Key()
		if i, dup := index[key]; dup {
			if c.URL != "" && c.URL != out[i].URL {
				out[i].AltURLs = append(out[i].AltURLs, c.URL)
			}
			continue
		}
		index[key] = len(out)
		out = append(out, c)
	}
	return out
}
