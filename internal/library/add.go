package library

import (
	"context"
	"fmt"
	"net/url"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// Overrides are corrections made by the user in the preview (RF-02).
// Empty fields keep the extracted value.
type Overrides struct {
	Title    string            `json:"title,omitempty"`
	Type     domain.SeriesType `json:"type,omitempty"`
	CoverURL string            `json:"cover_url,omitempty"`
	Category domain.Category   `json:"category,omitempty"`
}

func (o Overrides) validate() error {
	if o.Type != "" && !o.Type.Valid() {
		return fmt.Errorf("type %q: %w", o.Type, domain.ErrInvalidInput)
	}
	if o.Category != "" && !o.Category.Valid() {
		return fmt.Errorf("category %q: %w", o.Category, domain.ErrInvalidInput)
	}
	return nil
}

// Add extracts rawURL and saves it as a new series (RF-01). It fails with
// domain.ErrDuplicate when the series is already in the library (RF-04),
// checked both before extracting and atomically when saving.
func (s *Service) Add(ctx context.Context, rawURL string, o Overrides) (domain.Series, error) {
	if err := o.validate(); err != nil {
		return domain.Series{}, err
	}
	u, err := extract.NormalizeURL(rawURL)
	if err != nil {
		return domain.Series{}, err
	}
	if err := checkDuplicate(s.store.Library(), u); err != nil {
		return domain.Series{}, err
	}
	res, err := s.extractor.Extract(ctx, u.String())
	if err != nil {
		return domain.Series{}, fmt.Errorf("add %s: %w", u, err)
	}
	series, err := s.newSeries(res, o)
	if err != nil {
		return domain.Series{}, err
	}
	sourceURL, err := url.Parse(series.Source.URL)
	if err != nil {
		return domain.Series{}, fmt.Errorf("add %s: source url: %w", u, domain.ErrInvalidInput)
	}
	err = s.store.Update(ctx, func(lib *storage.Library) error {
		if err := checkDuplicate(lib, u, sourceURL); err != nil {
			return err
		}
		lib.Series = append(lib.Series, series)
		return nil
	})
	if err != nil {
		return domain.Series{}, err
	}
	s.syncCover(ctx, series.ID)
	s.afterSave(ctx, series.ID)
	return s.Get(series.ID)
}

// newSeries builds a library series from an extraction. Chapters imported
// here have no discovered_at: they were not seen being released.
func (s *Service) newSeries(res *extract.Result, o Overrides) (domain.Series, error) {
	id, err := s.newID()
	if err != nil {
		return domain.Series{}, err
	}
	now := s.clock.Now()
	info := res.Series
	chapters := make([]domain.Chapter, 0, len(res.Chapters))
	for _, c := range res.Chapters {
		chapters = append(chapters, fromInfo(c))
	}
	series := domain.Series{
		ID:        id,
		Type:      firstNonEmpty(o.Type, info.Type, domain.TypeOther),
		Title:     firstNonEmpty(o.Title, info.Title),
		AltTitles: nonNil(info.AltTitles),
		Source:    sourceOf(info.URL, res.Method),
		Cover:     domain.Cover{RemoteURL: firstNonEmpty(o.CoverURL, info.CoverURL)},
		Status:    info.Status,
		StatusRaw: info.StatusRaw,
		Synopsis:  info.Synopsis,
		Tags:      []string{},
		Category:  firstNonEmpty(o.Category, domain.CategoryReading),
		AddedAt:   now,
		Chapters:  chapters,
	}
	series.LastCheckedAt = &now
	series.RecomputeDerived()
	series.SeenLatestChapter = series.LatestChapter // nothing is "new" on add
	return series, nil
}

// checkDuplicate fails when any of urls matches the source URL or a mirror
// of a series already in lib (same extract.CanonicalKey).
func checkDuplicate(lib *storage.Library, urls ...*url.URL) error {
	want := map[string]bool{}
	for _, u := range urls {
		want[extract.CanonicalKey(u)] = true
	}
	for _, s := range lib.Series {
		for _, raw := range append([]string{s.Source.URL}, s.Source.Mirrors...) {
			u, err := url.Parse(raw)
			if err == nil && want[extract.CanonicalKey(u)] {
				return fmt.Errorf("series %q (%s): %w", s.Title, s.ID, domain.ErrDuplicate)
			}
		}
	}
	return nil
}

func sourceOf(rawURL, method string) domain.Source {
	src := domain.Source{URL: rawURL, Method: method, Mirrors: []string{}}
	if u, err := url.Parse(rawURL); err == nil {
		src.Domain = extract.Domain(u)
	}
	return src
}

func firstNonEmpty[T ~string](vs ...T) T {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
