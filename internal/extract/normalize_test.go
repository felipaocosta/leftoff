package extract

import (
	"errors"
	"net/url"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

func num(v float64) *float64 { return &v }

func TestNormalizeChapters(t *testing.T) {
	base, _ := url.Parse("https://a.org/manga/x/")
	in := []domain.ChapterInfo{
		{Number: num(2), Label: " Chapter  2 ", URL: "chapter-2/"},
		{Number: num(2), Label: "Chapter 2 (other scan)", URL: "https://b.org/x/2"},
		{Number: num(2), Label: "Chapter 2", URL: "chapter-2/"}, // same URL: not an alt
		{Number: num(1), URL: "chapter-1/"},                     // label filled in
		{Label: "Afterword", URL: "afterword/"},
		{URL: "junk/"}, // no label, no number: dropped
	}
	got := normalizeChapters(in, base)
	if len(got) != 3 {
		t.Fatalf("got %d chapters, want 3: %+v", len(got), got)
	}
	if got[0].Label != "Chapter 2" || got[0].URL != "https://a.org/manga/x/chapter-2/" {
		t.Errorf("first = %+v", got[0])
	}
	if len(got[0].AltURLs) != 1 || got[0].AltURLs[0] != "https://b.org/x/2" {
		t.Errorf("alt urls = %v", got[0].AltURLs)
	}
	if got[1].Label != "Chapter 1" {
		t.Errorf("generated label = %q", got[1].Label)
	}
	if got[2].Label != "Afterword" || got[2].Number != nil {
		t.Errorf("numberless chapter = %+v", got[2])
	}
}

func TestNormalizeResult(t *testing.T) {
	base, _ := url.Parse("https://a.org/manga/x/")
	r := &Result{
		Method: "adapter:test",
		Series: domain.SeriesInfo{Title: "  The  Title ", CoverURL: "/c.jpg", StatusRaw: " OnGoing "},
	}
	if err := normalizeResult(r, base); err != nil {
		t.Fatal(err)
	}
	s := r.Series
	if s.Title != "The Title" || s.CoverURL != "https://a.org/c.jpg" || s.Status != domain.StatusOngoing ||
		s.StatusRaw != "OnGoing" || s.Type != domain.TypeOther || s.URL != base.String() {
		t.Errorf("series = %+v", s)
	}
	if r.Confidence != weightTitle+weightCover+weightStatus {
		t.Errorf("confidence = %v", r.Confidence)
	}
}

func TestNormalizeResult_MissingTitleIsIncomplete(t *testing.T) {
	base, _ := url.Parse("https://a.org/")
	err := normalizeResult(&Result{Series: domain.SeriesInfo{Title: "   "}}, base)
	if !errors.Is(err, ErrIncomplete) {
		t.Errorf("err = %v, want ErrIncomplete", err)
	}
}

func TestConfidence(t *testing.T) {
	full := domain.SeriesInfo{Title: "t", CoverURL: "c", Status: domain.StatusOngoing}
	tests := []struct {
		name     string
		info     domain.SeriesInfo
		chapters []domain.ChapterInfo
		want     float64
	}{
		{"everything", full, []domain.ChapterInfo{{Number: num(1)}}, 1},
		{"numberless chapters count half", full, []domain.ChapterInfo{{Label: "Extra"}}, 0.75},
		{"no chapters", full, nil, 0.5},
		{"unknown status", domain.SeriesInfo{Title: "t", Status: domain.StatusUnknown}, nil, 0.3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Confidence(tt.info, tt.chapters); got < tt.want-1e-9 || got > tt.want+1e-9 {
				t.Errorf("Confidence = %v, want %v", got, tt.want)
			}
		})
	}
}
