package library

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// RefreshReport is the outcome of refreshing one series.
type RefreshReport struct {
	SeriesID    string `json:"series_id"`
	NewChapters int    `json:"new_chapters"`
	// Added are the new chapters found by this refresh, as stored, for
	// notifications (RF-26).
	Added []domain.Chapter `json:"-"`
	Err   error            `json:"-"`
}

// Refresh re-extracts a series and merges its chapters (RF-20). On failure
// the error is recorded in check_error (RF-24) and returned; read state and
// chapters are left untouched. User-editable fields (title, type, tags,
// notes, category) are never overwritten.
func (s *Service) Refresh(ctx context.Context, id string) (RefreshReport, error) {
	ctx = fetch.Fresh(ctx) // the site, not a copy from the last minutes (SPEC §5.8)
	current, err := s.Get(id)
	if err != nil {
		return RefreshReport{}, err
	}
	report := RefreshReport{SeriesID: id}
	res, err := s.extractor.Extract(ctx, current.Source.URL)
	if err != nil {
		return report, s.recordFailure(ctx, id, fmt.Errorf("refresh %s: %w", id, err))
	}

	var mergeErr error
	now := s.clock.Now()
	updated, err := s.updateSeries(ctx, id, func(sr *domain.Series) error {
		merged, err := mergeChapters(sr.Chapters, res.Chapters, now)
		if err != nil {
			mergeErr = err
			return err
		}
		sr.Chapters = merged.Chapters
		sr.Status, sr.StatusRaw = res.Series.Status, res.Series.StatusRaw
		if res.Series.Synopsis != "" {
			sr.Synopsis = res.Series.Synopsis
		}
		if res.Series.CoverURL != "" && !sr.Cover.Custom {
			sr.Cover.RemoteURL = res.Series.CoverURL
		}
		sr.Source.Method = res.Method
		// The site redirected to a new address (domain move, RF-08).
		migrateSource(sr, res.Series.URL, res.Method)
		sr.LastCheckedAt = &now
		sr.CheckError = nil
		if merged.New > 0 {
			sr.LastNewChapterAt = &now
		}
		report.NewChapters = merged.New
		return nil
	})
	if mergeErr != nil {
		return report, s.recordFailure(ctx, id, fmt.Errorf("refresh %s: %w", id, mergeErr))
	}
	if err != nil {
		return report, err
	}
	report.Added = addedAt(updated.Chapters, now)
	if after, _ := s.Get(id); !after.Cover.Custom && (after.Cover.RemoteURL != current.Cover.RemoteURL || after.Cover.Local == "") {
		s.syncCover(ctx, id)
	}
	s.afterSave(ctx, id)
	return report, nil
}

// RefreshAll refreshes every series one after the other (RF-21) and reports
// each result; one failure does not stop the others. Parallelism across
// domains belongs to the scheduler (SPEC §9).
func (s *Service) RefreshAll(ctx context.Context) []RefreshReport {
	series := s.List()
	reports := make([]RefreshReport, 0, len(series))
	for _, sr := range series {
		if ctx.Err() != nil {
			reports = append(reports, RefreshReport{SeriesID: sr.ID, Err: ctx.Err()})
			continue
		}
		r, err := s.Refresh(ctx, sr.ID)
		r.SeriesID, r.Err = sr.ID, err
		reports = append(reports, r)
	}
	return reports
}

// recordFailure stores cause in the series' check_error, counting
// consecutive failures, and returns cause. If even that write fails, both
// errors are returned.
func (s *Service) recordFailure(ctx context.Context, id string, cause error) error {
	_, err := s.updateSeries(ctx, id, func(sr *domain.Series) error {
		now := s.clock.Now()
		failures := 1
		if sr.CheckError != nil {
			failures = sr.CheckError.Failures + 1
		}
		sr.CheckError = &domain.CheckError{Code: checkErrorCode(cause), Message: cause.Error(), At: now, Failures: failures}
		sr.LastCheckedAt = &now
		return nil
	})
	if err != nil {
		return errors.Join(cause, fmt.Errorf("record check error: %w", err))
	}
	return cause
}

// addedAt returns the chapters discovered at t (new in this refresh).
func addedAt(chapters []domain.Chapter, t time.Time) []domain.Chapter {
	var out []domain.Chapter
	for _, c := range chapters {
		if c.DiscoveredAt != nil && c.DiscoveredAt.Equal(t) {
			out = append(out, c)
		}
	}
	return out
}
