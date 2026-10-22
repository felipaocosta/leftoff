package library

import (
	"context"
	"fmt"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// MarkRead sets the read state of the chapters with the given keys (RF-10).
// Unknown keys fail with domain.ErrNotFound and change nothing.
func (s *Service) MarkRead(ctx context.Context, id string, keys []string, read bool) (domain.Series, error) {
	return s.updateSeries(ctx, id, func(sr *domain.Series) error {
		index := make(map[string]int, len(sr.Chapters))
		for i, c := range sr.Chapters {
			index[c.Key] = i
		}
		for _, k := range keys {
			if _, ok := index[k]; !ok {
				return fmt.Errorf("chapter %q of %s: %w", k, id, domain.ErrNotFound)
			}
		}
		for _, k := range keys {
			s.setRead(&sr.Chapters[index[k]], read)
		}
		return nil
	})
}

// MarkReadUpTo marks every numbered chapter up to and including upTo as
// read (RF-11). Chapters without a number are left as they are, since
// their place in the reading order is unknown.
func (s *Service) MarkReadUpTo(ctx context.Context, id string, upTo float64) (domain.Series, error) {
	return s.updateSeries(ctx, id, func(sr *domain.Series) error {
		for i := range sr.Chapters {
			if n := sr.Chapters[i].Number; n != nil && *n <= upTo {
				s.setRead(&sr.Chapters[i], true)
			}
		}
		return nil
	})
}

// MarkSeen clears the "new chapters" flag of a series (RF-23).
func (s *Service) MarkSeen(ctx context.Context, id string) (domain.Series, error) {
	return s.updateSeries(ctx, id, func(sr *domain.Series) error {
		sr.RecomputeDerived()
		sr.SeenLatestChapter = sr.LatestChapter
		return nil
	})
}

// setRead changes one chapter, keeping the original read_at of chapters
// that were already read.
func (s *Service) setRead(c *domain.Chapter, read bool) {
	switch {
	case read && !c.Read:
		now := s.clock.Now()
		c.Read, c.ReadAt = true, &now
	case !read:
		c.Read, c.ReadAt = false, nil
	}
}
