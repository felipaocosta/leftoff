package heuristic

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Real pages that only the heuristic handles (no adapter), captured on
// 2026-10-19 (see testdata/fixtures/<domain>/README.md).
func TestExtract_RealFixtures(t *testing.T) {
	tests := []struct {
		name, file, url string
		title           string
		status          domain.Status
		typ             domain.SeriesType
		chapters        int
		maxChapter      float64
		minDated        int    // chapters with a release date (relative dates read)
		coverHas        string // part of the expected cover URL
	}{
		{
			name:  "vortexscans: labels next to relative dates",
			file:  "../../../testdata/fixtures/vortexscans.org/survival-supremacy.html",
			url:   "https://vortexscans.org/series/survival-supremacy",
			title: "Survival Supremacy", status: domain.StatusOngoing, typ: domain.TypeManhwa, // "Type: Manhwa" badge
			chapters: 16, maxChapter: 16, minDated: 10,
		},
		{
			name:  "comix.to: page rendered by FlareSolverr, one entry per scan group",
			file:  "../../../testdata/fixtures/comix.to/sonna-kazoku-nara-sutechaeba.html",
			url:   "https://comix.to/title/zdk1m-sonna-kazoku-nara-sutechaeba",
			title: "Sonna Kazoku nara Sutechaeba?", status: domain.StatusOngoing, typ: domain.TypeManga,
			chapters: 10, maxChapter: 37, // only the latest releases are on the page
		},
		{
			name:  "roliascan: decorated JSON-LD name, status in JSON-LD",
			file:  "../../../testdata/fixtures/roliascan.com/healing-at-the-academy-convenience-store.html",
			url:   "https://roliascan.com/manga/healing-at-the-academy-convenience-store/",
			title: "Healing at the Academy Convenience Store", status: domain.StatusOngoing, typ: domain.TypeManhwa,
			chapters: 1, maxChapter: 1, // the full list loads with JavaScript
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			html, err := testfixture.ReadFile(t, tt.file)
			if err != nil {
				t.Fatal(err)
			}
			f := &fetch.Static{Pages: map[string][]byte{tt.url: html}}
			captured := domain.FixedClock{T: time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)}
			res, err := extract.New(f, extract.Options{Fallback: New(captured)}).Extract(context.Background(), tt.url)
			if err != nil {
				t.Fatal(err)
			}
			s := res.Series
			if tt.coverHas != "" && !strings.Contains(s.CoverURL, tt.coverHas) {
				t.Errorf("cover = %q, want it to contain %q", s.CoverURL, tt.coverHas)
			}
			if s.Title != tt.title || s.Status != tt.status || s.Type != tt.typ {
				t.Errorf("series = %q %q %q", s.Title, s.Status, s.Type)
			}
			if len(res.Chapters) != tt.chapters {
				t.Errorf("chapters = %d, want %d", len(res.Chapters), tt.chapters)
			}
			max := -1.0
			for _, c := range res.Chapters {
				if c.Number == nil {
					t.Errorf("chapter without number: %+v", c)
					continue
				}
				if *c.Number > max {
					max = *c.Number
				}
				// The label must carry the number and nothing glued to it
				// ("Chapter 148 days" was the vortexscans bug).
				want := domain.ChapterKey(c.Number, "")
				if !regexp.MustCompile(`(?i)^(read )?(chapter |ch\.)` + regexp.QuoteMeta(want) + `$`).MatchString(c.Label) {
					t.Errorf("label %q does not match number %v", c.Label, *c.Number)
				}
			}
			if max != tt.maxChapter {
				t.Errorf("max chapter = %v, want %v", max, tt.maxChapter)
			}
			dated := 0
			for _, c := range res.Chapters {
				if !c.ReleasedAt.IsZero() {
					dated++
				}
			}
			if dated < tt.minDated {
				t.Errorf("%d chapters have a release date, want at least %d", dated, tt.minDated)
			}
		})
	}
}
