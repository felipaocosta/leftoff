package mgeko

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

var update = flag.Bool("update", false, "rewrite golden files")

const fixtureDir = "../../../../testdata/fixtures/mgeko.cc"

func TestAdapter_Fixtures(t *testing.T) {
	tests := []struct {
		slug, title  string
		typ          domain.SeriesType
		chapters     int
		maxChapter   float64
		latest       string
		minAltTitles int
	}{
		{"grand-finale", "Grand Finale", domain.TypeManhwa, 50, 50, "2026-09-14", 2},
		{"yuumei-v-tuber-no-ani-dakedo-nazeka-ore-ga-yuumei-ni-natteita",
			"I'm the Older Brother of a Famous VTuber, But for Some Reason I Became Famous", domain.TypeManga, 25, 24, "2026-09-16", 1},
	}
	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			read := func(n string) []byte {
				b, err := testfixture.ReadFile(t, filepath.Join(fixtureDir, n))
				if err != nil {
					t.Fatal(err)
				}
				return b
			}
			base := "https://www.mgeko.cc/manga/" + tt.slug + "/"
			f := &fetch.Static{Pages: map[string][]byte{
				base:                   read(tt.slug + ".html"),
				base + "all-chapters/": read(tt.slug + "-all-chapters.html"),
			}}
			res, err := extract.New(f, extract.Options{Adapters: []extract.Adapter{New()}}).Extract(context.Background(), base)
			if err != nil {
				t.Fatal(err)
			}
			s := res.Series
			if res.Method != "adapter:mgeko" || !strings.HasPrefix(s.Title, tt.title) || s.Status != domain.StatusOngoing ||
				s.Type != tt.typ || len(s.AltTitles) < tt.minAltTitles || s.Synopsis != "" ||
				!strings.Contains(s.CoverURL, "/manga_covers/") || strings.Contains(s.CoverURL, placeholderImg) {
				t.Errorf("series = %+v", s)
			}
			for _, alt := range s.AltTitles {
				if alt == "Updating" {
					t.Error(`"Updating" is a placeholder, not a title`)
				}
			}
			if len(res.Chapters) != tt.chapters {
				t.Fatalf("chapters = %d, want %d", len(res.Chapters), tt.chapters)
			}
			max := -1.0
			for _, c := range res.Chapters {
				if c.Number != nil && *c.Number > max {
					max = *c.Number
				}
				if c.ReleasedAt.IsZero() {
					t.Errorf("chapter %q has no date", c.Label)
				}
			}
			if max != tt.maxChapter || res.Chapters[0].ReleasedAt.String() != tt.latest ||
				!strings.HasPrefix(res.Chapters[0].Label, "Chapter ") {
				t.Errorf("max = %v, first = %+v", max, res.Chapters[0])
			}
			checkGolden(t, filepath.Join(fixtureDir, tt.slug+".golden.json"), res)
		})
	}
}

func TestAdapter_Match(t *testing.T) {
	a := New()
	for raw, want := range map[string]bool{
		"https://www.mgeko.cc/manga/grand-finale/":                       true,
		"https://mgeko.com/manga/grand-finale":                           true,
		"https://www.mgeko.cc/manga/grand-finale/all-chapters/":          false,
		"https://www.mgeko.cc/reader/en/grand-finale-chapter-50-eng-li/": false,
		"https://other.org/manga/x/":                                     false,
	} {
		u, _ := url.Parse(raw)
		if got := a.Match(u, nil); got != want {
			t.Errorf("Match(%s) = %v, want %v", raw, got, want)
		}
	}
	u, _ := url.Parse("https://mirror.example/manga/x/")
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(`<h1 class="novel-title">X</h1><a href="/manga/x/all-chapters/">all</a>`))
	if !a.Match(u, doc) {
		t.Error("site markup should match")
	}
}

func TestParseDate(t *testing.T) {
	for in, want := range map[string]string{
		"Sept. 14, 2026, 2:18 a.m.": "2026-09-14",
		"May 12, 2025, 5:01 a.m.":   "2025-05-12",
		"June 3, 2026, noon":        "2026-06-03",
		"March 30, 2026":            "2026-03-30",
		"1 month ago":               "",
		"Foo. 1, 2026":              "",
	} {
		if got := parseDate(in).String(); got != want {
			t.Errorf("parseDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func checkGolden(t *testing.T, path string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("result differs from %s; run go test ./... -update and review the diff", path)
	}
}
