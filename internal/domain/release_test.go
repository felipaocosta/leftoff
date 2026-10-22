package domain

import (
	"testing"
	"time"
)

// dated builds chapters released on the given dates ("YYYY-MM-DD"; "" means
// unknown), newest first like the sources list them.
func dated(t *testing.T, dates ...string) []Chapter {
	t.Helper()
	chs := make([]Chapter, len(dates))
	for i, s := range dates {
		if s == "" {
			continue
		}
		d, err := ParseDate(s)
		if err != nil {
			t.Fatal(err)
		}
		chs[i] = Chapter{ReleasedAt: d}
	}
	return chs
}

func TestReleaseInterval(t *testing.T) {
	tests := []struct {
		name   string
		dates  []string
		want   float64
		wantOK bool
	}{
		{"weekly", []string{"2026-10-15", "2026-10-08", "2026-10-01", "2026-09-24"}, 7, true},
		{"batch releases count once", []string{"2026-10-15", "2026-10-15", "2026-10-01", "2026-10-01", "2026-09-17"}, 14, true},
		{"median ignores one long delay", []string{"2026-10-29", "2026-10-22", "2026-09-01", "2026-08-25", "2026-08-18"}, 7, true},
		{"even count averages middle gaps", []string{"2026-10-20", "2026-10-15", "2026-10-09", "2026-10-03", "2026-09-26"}, 6, true},
		{"unknown dates are ignored", []string{"", "2026-10-15", "", "2026-10-08", "2026-10-01"}, 7, true},
		{"too few dates", []string{"2026-10-15", "2026-10-08", "", ""}, 0, false},
		{"no dates", []string{"", ""}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ReleaseInterval(dated(t, tt.dates...))
			if ok != tt.wantOK || got != tt.want {
				t.Errorf("ReleaseInterval = %v, %v; want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestReleaseInterval_UsesOnlyRecentReleases(t *testing.T) {
	// 20 monthly releases, then 10 weekly ones: the recent weekly schedule wins.
	var dates []string
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 20 {
		dates = append(dates, start.AddDate(0, i, 0).Format(dateLayout))
	}
	last := start.AddDate(0, 19, 0)
	for i := 1; i <= 10; i++ {
		dates = append(dates, last.AddDate(0, 0, 7*i).Format(dateLayout))
	}
	if got, ok := ReleaseInterval(dated(t, dates...)); !ok || got != 7 {
		t.Errorf("ReleaseInterval = %v, %v; want 7", got, ok)
	}
}

func TestNextReleaseEstimate(t *testing.T) {
	weekly := []string{"2026-10-15", "2026-10-08", "2026-10-01"}
	tests := []struct {
		name   string
		status Status
		dates  []string
		want   string
	}{
		{"ongoing", StatusOngoing, weekly, "2026-10-22"},
		{"unknown status still estimates", StatusUnknown, weekly, "2026-10-22"},
		{"empty status still estimates", "", weekly, "2026-10-22"},
		{"interval is rounded to whole days", StatusOngoing, []string{"2026-10-20", "2026-10-15", "2026-10-09", "2026-10-03", "2026-09-26"}, "2026-10-26"},
		{"completed has no next", StatusCompleted, weekly, ""},
		{"hiatus has no next", StatusHiatus, weekly, ""},
		{"cancelled has no next", StatusCancelled, weekly, ""},
		{"not enough data", StatusOngoing, []string{"2026-10-15"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NextReleaseEstimate(tt.status, dated(t, tt.dates...))
			if got.String() != tt.want || ok != (tt.want != "") {
				t.Errorf("NextReleaseEstimate = %q, %v; want %q", got, ok, tt.want)
			}
		})
	}
}

func TestSeries_RecomputeDerived_ReleaseFields(t *testing.T) {
	s := Series{Status: StatusOngoing, Chapters: dated(t, "2026-10-20", "2026-10-15", "2026-10-09", "2026-10-03")}
	s.RecomputeDerived()
	if s.ReleaseIntervalDays == nil || *s.ReleaseIntervalDays != 6 || s.NextReleaseEstimate.String() != "2026-10-26" {
		t.Errorf("interval = %v, next = %q", s.ReleaseIntervalDays, s.NextReleaseEstimate)
	}

	s.Status = StatusCompleted
	s.RecomputeDerived()
	if s.ReleaseIntervalDays == nil || !s.NextReleaseEstimate.IsZero() {
		t.Errorf("completed: interval should stay, next should clear; got %v, %q", s.ReleaseIntervalDays, s.NextReleaseEstimate)
	}

	s.Chapters = nil
	s.RecomputeDerived()
	if s.ReleaseIntervalDays != nil {
		t.Error("interval should clear without dates")
	}
}

func TestReleaseInterval_RoundsToOneDecimalInSeries(t *testing.T) {
	// Gaps 3 and 4 days → median 3.5.
	s := Series{Status: StatusOngoing, Chapters: dated(t, "2026-10-08", "2026-10-04", "2026-10-01")}
	s.RecomputeDerived()
	if s.ReleaseIntervalDays == nil || *s.ReleaseIntervalDays != 3.5 || s.NextReleaseEstimate.String() != "2026-10-12" {
		t.Errorf("interval = %v, next = %q", s.ReleaseIntervalDays, s.NextReleaseEstimate)
	}
}

func TestChapter_ReleaseDate(t *testing.T) {
	discovered := time.Date(2026, 10, 19, 23, 30, 0, 0, time.FixedZone("BRT", -3*3600)) // 02:30 UTC next day
	tests := []struct {
		name string
		ch   Chapter
		want string
	}{
		{"source date wins", Chapter{ReleasedAt: NewDate(2026, 10, 1), DiscoveredAt: &discovered}, "2026-10-01"},
		{"discovery date in UTC", Chapter{DiscoveredAt: &discovered}, "2026-10-20"},
		{"nothing known", Chapter{}, ""},
	}
	for _, tt := range tests {
		if got := tt.ch.ReleaseDate().String(); got != tt.want {
			t.Errorf("%s: ReleaseDate = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestReleaseInterval_UsesDiscoveryWhenSourceHasNoDates(t *testing.T) {
	at := func(d string) *time.Time {
		v, _ := time.Parse(dateLayout, d)
		return &v
	}
	chs := []Chapter{
		{DiscoveredAt: at("2026-10-15")},
		{DiscoveredAt: at("2026-10-08")},
		{DiscoveredAt: at("2026-10-01")},
		{}, {}, // imported when the series was added: no date at all
	}
	if got, ok := ReleaseInterval(chs); !ok || got != 7 {
		t.Errorf("ReleaseInterval = %v, %v; want 7", got, ok)
	}
}
