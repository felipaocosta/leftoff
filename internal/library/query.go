package library

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// Sort orders for Query (RF-34).
const (
	SortUpdated = "updated" // most recent new chapter first (default)
	SortTitle   = "title"   // alphabetical
	SortUnread  = "unread"  // most unread chapters first
	SortAdded   = "added"   // most recently added first
)

// Query filters and sorts the library (RF-32, RF-33, RF-34). Empty fields
// do not filter.
type Query struct {
	Text     string // matches title and alternative titles, case-insensitive
	Status   domain.Status
	Category domain.Category
	Type     domain.SeriesType
	HasNew   *bool
	Source   string // the series' current site (source.domain), e.g. "mangaread.org"
	Sort     string
}

func (q Query) validate() error {
	switch q.Sort {
	case "", SortUpdated, SortTitle, SortUnread, SortAdded:
		return nil
	}
	return fmt.Errorf("sort %q: %w", q.Sort, domain.ErrInvalidInput)
}

// Find returns the series matching q, sorted. The returned slice is new,
// but its elements share chapter slices with the snapshot: do not modify.
func (s *Service) Find(q Query) ([]domain.Series, error) {
	if err := q.validate(); err != nil {
		return nil, err
	}
	text := strings.ToLower(strings.TrimSpace(q.Text))
	q.Source = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(q.Source)), "www.")
	out := []domain.Series{}
	for _, sr := range s.List() {
		if q.matches(&sr, text) {
			out = append(out, sr)
		}
	}
	slices.SortStableFunc(out, compareBy(q.Sort))
	return out, nil
}

// SourceCount is one site of the library and how many series use it.
type SourceCount struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

// Sources lists the sites the series are read from (their current
// domain), alphabetically, for the source filter (RF-33).
func (s *Service) Sources() []SourceCount {
	counts := map[string]int{}
	for _, sr := range s.List() {
		if d := strings.TrimPrefix(strings.ToLower(sr.Source.Domain), "www."); d != "" {
			counts[d]++
		}
	}
	out := make([]SourceCount, 0, len(counts))
	for d, n := range counts {
		out = append(out, SourceCount{Domain: d, Count: n})
	}
	slices.SortFunc(out, func(a, b SourceCount) int { return cmp.Compare(a.Domain, b.Domain) })
	return out
}

func (q Query) matches(sr *domain.Series, text string) bool {
	switch {
	case q.Status != "" && sr.Status != q.Status,
		q.Category != "" && sr.Category != q.Category,
		q.Type != "" && sr.Type != q.Type,
		q.HasNew != nil && sr.HasNew() != *q.HasNew,
		q.Source != "" && strings.TrimPrefix(strings.ToLower(sr.Source.Domain), "www.") != q.Source:
		return false
	}
	if text == "" {
		return true
	}
	for _, t := range append([]string{sr.Title}, sr.AltTitles...) {
		if strings.Contains(strings.ToLower(t), text) {
			return true
		}
	}
	return false
}

func compareBy(sort string) func(a, b domain.Series) int {
	byTitle := func(a, b domain.Series) int { return cmp.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title)) }
	switch sort {
	case SortTitle:
		return byTitle
	case SortUnread:
		return func(a, b domain.Series) int {
			return cmp.Or(cmp.Compare(b.UnreadCount(), a.UnreadCount()), byTitle(a, b))
		}
	case SortAdded:
		return func(a, b domain.Series) int { return cmp.Or(b.AddedAt.Compare(a.AddedAt), byTitle(a, b)) }
	default: // SortUpdated: series that never got a new chapter go last
		return func(a, b domain.Series) int {
			return cmp.Or(compareTimeDesc(a.LastNewChapterAt, b.LastNewChapterAt), byTitle(a, b))
		}
	}
}

// compareTimeDesc orders newer times first and nil last.
func compareTimeDesc(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return b.Compare(*a)
}
