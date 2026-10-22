package domain

import "testing"

func TestSeries_RecomputeDerived(t *testing.T) {
	tests := []struct {
		name          string
		status        Status
		chapters      []Chapter
		wantLatest    *float64
		wantLastRead  *float64
		wantTotal     *int
		wantUnreadCnt int
	}{
		{
			name:   "mixed read state",
			status: StatusOngoing,
			chapters: []Chapter{
				{Number: num(3), Read: false},
				{Number: num(2), Read: true},
				{Number: num(1), Read: true},
			},
			wantLatest: num(3), wantLastRead: num(2), wantUnreadCnt: 1,
		},
		{
			name:   "chapters without number are ignored for numbers",
			status: StatusOngoing,
			chapters: []Chapter{
				{Label: "Extra", Read: true},
				{Number: num(1.5), Read: false},
			},
			wantLatest: num(1.5), wantUnreadCnt: 1,
		},
		{
			name:       "completed sets total",
			status:     StatusCompleted,
			chapters:   []Chapter{{Number: num(1), Read: true}, {Number: num(2), Read: true}},
			wantLatest: num(2), wantLastRead: num(2), wantTotal: intPtr(2),
		},
		{name: "no chapters", status: StatusUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Series{Status: tt.status, Chapters: tt.chapters, TotalChapters: intPtr(99)}
			s.RecomputeDerived()
			assertFloatPtr(t, "LatestChapter", s.LatestChapter, tt.wantLatest)
			assertFloatPtr(t, "LastReadChapter", s.LastReadChapter, tt.wantLastRead)
			assertIntPtr(t, "TotalChapters", s.TotalChapters, tt.wantTotal)
			if got := s.UnreadCount(); got != tt.wantUnreadCnt {
				t.Errorf("UnreadCount = %d, want %d", got, tt.wantUnreadCnt)
			}
		})
	}
}

func TestSeriesType_Valid(t *testing.T) {
	for _, v := range []SeriesType{TypeManga, TypeManhwa, TypeManhua, TypeNovel, TypeOther} {
		if !v.Valid() {
			t.Errorf("%q should be valid", v)
		}
	}
	if SeriesType("comic").Valid() {
		t.Error(`"comic" should be invalid`)
	}
}

func TestCategory_Valid(t *testing.T) {
	for _, v := range []Category{CategoryReading, CategoryPlanToRead, CategoryOnHold, CategoryDropped, CategoryCompleted} {
		if !v.Valid() {
			t.Errorf("%q should be valid", v)
		}
	}
	if Category("").Valid() {
		t.Error("empty category should be invalid")
	}
}

func intPtr(v int) *int { return &v }

func assertFloatPtr(t *testing.T, field string, got, want *float64) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v", field, got, want)
	case *got != *want:
		t.Errorf("%s = %v, want %v", field, *got, *want)
	}
}

func assertIntPtr(t *testing.T, field string, got, want *int) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v", field, got, want)
	case *got != *want:
		t.Errorf("%s = %d, want %d", field, *got, *want)
	}
}

func TestSeries_HasNew(t *testing.T) {
	tests := []struct {
		name         string
		latest, seen *float64
		want         bool
	}{
		{"no chapters", nil, nil, false},
		{"never seen", num(3), nil, true},
		{"newer than seen", num(4), num(3), true},
		{"up to date", num(3), num(3), false},
	}
	for _, tt := range tests {
		s := Series{LatestChapter: tt.latest, SeenLatestChapter: tt.seen}
		if got := s.HasNew(); got != tt.want {
			t.Errorf("%s: HasNew = %v, want %v", tt.name, got, tt.want)
		}
	}
}
