package liliana

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

const fixtureDir = "../../../../testdata/fixtures/manhuaplus.org"

func TestAdapter_Fixtures(t *testing.T) {
	tests := []struct {
		fixture     string
		url         string
		title       string
		cover       string
		status      domain.Status
		typ         domain.SeriesType
		chapters    int
		maxChapter  float64
		minAltTitle int
	}{
		{
			fixture: "forging-an-industrial-empire-in-a-magical-world",
			url:     "https://manhuaplus.org/manga/forging-an-industrial-empire-in-a-magical-world",
			title:   "Forging an Industrial Empire in a Magical World",
			cover:   "https://manhuaplus.org/uploads/covers/forging-an-industrial-empire-in-a-magical-world.jpg",
			status:  domain.StatusOngoing, typ: domain.TypeManhua,
			chapters: 81, maxChapter: 80, minAltTitle: 2,
		},
		{
			fixture: "catastrophic-necromancer001",
			url:     "https://manhuaplus.org/manga/catastrophic-necromancer001",
			title:   "Necromancer, the Ultimate Scourge!",
			cover:   "https://manhuaplus.org/uploads/covers/I-Am-a-Disaster.jpg?1705216046",
			status:  domain.StatusOngoing, typ: domain.TypeManhwa, // as labelled by the site
			chapters: 316, maxChapter: 315, minAltTitle: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			html, err := testfixture.ReadFile(t, filepath.Join(fixtureDir, tt.fixture+".html"))
			if err != nil {
				t.Fatal(err)
			}
			f := &fetch.Static{Pages: map[string][]byte{tt.url: html}}
			res, err := extract.New(f, extract.Options{Adapters: []extract.Adapter{New()}}).Extract(context.Background(), tt.url)
			if err != nil {
				t.Fatal(err)
			}
			s := res.Series
			if res.Method != "adapter:liliana" || s.Title != tt.title || s.CoverURL != tt.cover ||
				s.Status != tt.status || s.Type != tt.typ || len(s.AltTitles) < tt.minAltTitle {
				t.Errorf("series = %+v (method %s)", s, res.Method)
			}
			if len(res.Chapters) != tt.chapters {
				t.Errorf("chapters = %d, want %d", len(res.Chapters), tt.chapters)
			}
			if got := maxNumber(res.Chapters); got != tt.maxChapter {
				t.Errorf("max chapter = %v, want %v", got, tt.maxChapter)
			}
			checkGolden(t, filepath.Join(fixtureDir, tt.fixture+".golden.json"), res)
		})
	}
}

func TestAdapter_Match(t *testing.T) {
	a := New()
	urlOnly := []struct {
		url  string
		want bool
	}{
		{"https://manhuaplus.org/manga/x", true},
		{"https://www.manhuaplus.org/manga/x/", true},
		{"https://manhuaplus.org/manga/x/chapter-80", false},
		{"https://other.org/manga/x", false},
	}
	for _, tt := range urlOnly {
		u, _ := url.Parse(tt.url)
		if got := a.Match(u, nil); got != tt.want {
			t.Errorf("Match(%s, nil) = %v, want %v", tt.url, got, tt.want)
		}
	}
	u, _ := url.Parse("https://mirror.example/manga/x")
	theme := `<div class="y6x11p">Status <span class="dt">OnGoing</span></div><ul id="myUL"><li class="chapter"><a href="c1">Chapter 1</a></li></ul>`
	if !a.Match(u, mustDoc(t, theme)) {
		t.Error("theme markup should match")
	}
	if a.Match(u, mustDoc(t, `<ul class="chapters"><li><a href="c1">Chapter 1</a></li></ul>`)) {
		t.Error("unrelated markup should not match")
	}
}

func TestAdapter_MissingTitleIsIncomplete(t *testing.T) {
	u, _ := url.Parse("https://manhuaplus.org/manga/x")
	f := &fetch.Static{Pages: map[string][]byte{u.String(): []byte(`<div class="y6x11p"></div>`)}}
	if _, err := New().FetchSeries(context.Background(), f, u); err == nil || !strings.Contains(err.Error(), "required field") {
		t.Errorf("err = %v, want ErrIncomplete", err)
	}
}

func TestHelpers(t *testing.T) {
	if d := unixDate("1775829819"); d.String() != "2026-04-10" {
		t.Errorf("unixDate = %q", d)
	}
	for _, s := range []string{"", "abc", "0"} {
		if !unixDate(s).IsZero() {
			t.Errorf("unixDate(%q) should be zero", s)
		}
	}
	if seriesType("Manhwa") != domain.TypeManhwa || seriesType("Webtoon") != domain.TypeOther {
		t.Error("seriesType mapping")
	}
	if New("Mirror.org").Match(mustURL("https://mirror.org/manga/x"), nil) != true {
		t.Error("custom host should match")
	}
}

func maxNumber(chs []domain.ChapterInfo) float64 {
	m := -1.0
	for _, c := range chs {
		if c.Number != nil && *c.Number > m {
			m = *c.Number
		}
	}
	return m
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

func mustDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	d, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mustURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}

func TestFetchChapters_SkipsItemsWithoutALink(t *testing.T) {
	u, _ := url.Parse("https://manhuaplus.org/manga/x")
	page := `<ul id="myUL"><li class="chapter"><a href="https://manhuaplus.org/manga/x/chapter-2">Chapter 2</a></li>
		<li class="chapter"><span>Chapter 1</span></li></ul>`
	chs, err := New().FetchChapters(context.Background(), &fetch.Static{Pages: map[string][]byte{u.String(): []byte(page)}}, u)
	if err != nil || len(chs) != 1 || *chs[0].Number != 2 {
		t.Errorf("chapters = %+v, %v", chs, err)
	}
}
