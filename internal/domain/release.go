package domain

import (
	"math"
	"slices"
	"time"
)

const (
	// minReleaseDates is how many distinct release dates are needed before a
	// release interval is estimated (2 intervals).
	minReleaseDates = 3
	// maxReleaseIntervals limits the estimate to recent releases, so an old
	// schedule change does not skew it.
	maxReleaseIntervals = 10
	day                 = 24 * time.Hour
)

// ReleaseInterval estimates how many days pass between releases (RF-27): the
// median gap between the distinct release dates of the most recent
// chapters (Chapter.ReleaseDate: the source date, else the discovery date).
// Several chapters released on the same day count as one release.
// It returns false when fewer than minReleaseDates dates are known.
func ReleaseInterval(chapters []Chapter) (float64, bool) {
	dates := distinctReleaseDates(chapters)
	if len(dates) < minReleaseDates {
		return 0, false
	}
	if len(dates) > maxReleaseIntervals+1 {
		dates = dates[len(dates)-maxReleaseIntervals-1:]
	}
	gaps := make([]float64, 0, len(dates)-1)
	for i := 1; i < len(dates); i++ {
		gaps = append(gaps, dates[i].Sub(dates[i-1]).Hours()/24)
	}
	return median(gaps), true
}

// NextReleaseEstimate predicts the next release date: the latest known
// release plus the estimated interval. Only ongoing (or unknown status)
// series get an estimate; completed, cancelled and hiatus series do not.
func NextReleaseEstimate(status Status, chapters []Chapter) (Date, bool) {
	if status != StatusOngoing && status != StatusUnknown && status != "" {
		return Date{}, false
	}
	interval, ok := ReleaseInterval(chapters)
	if !ok {
		return Date{}, false
	}
	dates := distinctReleaseDates(chapters)
	next := dates[len(dates)-1].Add(time.Duration(math.Round(interval)) * day)
	return NewDate(next.Year(), next.Month(), next.Day()), true
}

// recomputeRelease refreshes the release fields of s (see RecomputeDerived).
func (s *Series) recomputeRelease() {
	s.ReleaseIntervalDays = nil
	if interval, ok := ReleaseInterval(s.Chapters); ok {
		rounded := math.Round(interval*10) / 10
		s.ReleaseIntervalDays = &rounded
	}
	s.NextReleaseEstimate, _ = NextReleaseEstimate(s.Status, s.Chapters)
}

// distinctReleaseDates returns the known release dates (see
// Chapter.ReleaseDate), unique and sorted from oldest to newest.
func distinctReleaseDates(chapters []Chapter) []time.Time {
	seen := map[Date]bool{}
	var dates []time.Time
	for _, c := range chapters {
		d := c.ReleaseDate()
		if d.IsZero() || seen[d] {
			continue
		}
		seen[d] = true
		dates = append(dates, d.t)
	}
	slices.SortFunc(dates, func(a, b time.Time) int { return a.Compare(b) })
	return dates
}

func median(xs []float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}
