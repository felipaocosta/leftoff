package madara

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

var update = flag.Bool("update", false, "rewrite golden files")

const fixtureDir = "../../../../testdata/fixtures/mangaread.org"

// capturedAt is when the fixtures were saved; relative dates ("3 hours
// ago") are counted from it.
var clock = domain.FixedClock{T: time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)}

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := testfixture.ReadFile(t, filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAdapter_Fixtures(t *testing.T) {
	tests := []struct {
		fixture    string
		url        string
		title      string
		cover      string
		status     domain.Status
		altTitles  []string
		chapters   int
		maxChapter float64
		latestDate string
		oldestDate string
	}{
		{
			fixture: "eleceed", url: "https://www.mangaread.org/manga/eleceed/",
			title:  "Eleceed",
			cover:  "https://www.mangaread.org/wp-content/uploads/2021/09/1615657818_198x0-193x278.jpg",
			status: domain.StatusOngoing, altTitles: []string{"Элисед", "일렉시드"},
			chapters: 423, maxChapter: 423, latestDate: "2026-10-20", // "3 hours ago"
		},
		{
			fixture: "hunter-academys-strongest-battle-god", url: "https://www.mangaread.org/manga/hunter-academys-strongest-battle-god/",
			title:  "Hunter Academy’s Strongest Battle God",
			cover:  "https://www.mangaread.org/wp-content/uploads/2024/04/Hunter-Academys-Strongest-Battle-God-193x278-1-193x278.webp",
			status: domain.StatusCompleted, altTitles: []string{"Hunter Academy's Greatest War God", "Hunter Academy's Battle God"},
			chapters: 41, maxChapter: 40.5, latestDate: "2025-07-17", oldestDate: "2024-04-13",
		},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			f := &fetch.Static{Pages: map[string][]byte{tt.url: read(t, tt.fixture+".html")}}
			res, err := extract.New(f, extract.Options{Adapters: []extract.Adapter{New(clock)}}).Extract(context.Background(), tt.url)
			if err != nil {
				t.Fatal(err)
			}
			s := res.Series
			if res.Method != "adapter:madara" || s.Title != tt.title || s.CoverURL != tt.cover || s.Status != tt.status ||
				s.Type != domain.TypeManhwa || strings.Join(s.AltTitles, "|") != strings.Join(tt.altTitles, "|") ||
				s.Synopsis == "" || strings.Contains(s.Synopsis, "Show more") {
				t.Errorf("series = %+v (method %s)", s, res.Method)
			}
			if len(res.Chapters) != tt.chapters {
				t.Errorf("chapters = %d, want %d", len(res.Chapters), tt.chapters)
			}
			if got := maxNumber(res.Chapters); got != tt.maxChapter {
				t.Errorf("max chapter = %v, want %v", got, tt.maxChapter)
			}
			if got := res.Chapters[0].ReleasedAt.String(); got != tt.latestDate {
				t.Errorf("latest release = %q, want %q", got, tt.latestDate)
			}
			if last := res.Chapters[len(res.Chapters)-1]; tt.oldestDate != "" && last.ReleasedAt.String() != tt.oldestDate {
				t.Errorf("oldest release = %q, want %q", last.ReleasedAt.String(), tt.oldestDate)
			}
			checkGolden(t, filepath.Join(fixtureDir, tt.fixture+".golden.json"), res)
		})
	}
}

func TestFetchChapters_AJAXEndpoint(t *testing.T) {
	u := mustURL("https://www.mangaread.org/manga/eleceed/")
	f := &fetch.Static{Pages: map[string][]byte{
		u.String(): read(t, "eleceed-ajax-page.html"),
		"POST https://www.mangaread.org/manga/eleceed/ajax/chapters/": read(t, "eleceed-ajax-chapters.html"),
	}}
	got, err := New(clock).FetchChapters(context.Background(), f, u)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 423 || got[0].Label != "Chapter 423" || got[0].URL != "https://www.mangaread.org/manga/eleceed/chapter-423/" {
		t.Errorf("%d chapters, first %+v", len(got), got[0])
	}
}

func TestFetchChapters_AdminAJAXFallback(t *testing.T) {
	u := mustURL("https://www.mangaread.org/manga/eleceed/")
	f := &fetch.Static{Pages: map[string][]byte{
		u.String(): read(t, "eleceed-ajax-page.html"),
		"POST https://www.mangaread.org/wp-admin/admin-ajax.php": read(t, "eleceed-ajax-chapters.html"),
	}}
	got, err := New(clock).FetchChapters(context.Background(), f, u)
	if err != nil || len(got) != 423 {
		t.Fatalf("%d chapters, %v", len(got), err)
	}
	if want := "POST https://www.mangaread.org/wp-admin/admin-ajax.php"; f.Requests[len(f.Requests)-1] != want {
		t.Errorf("requests = %v", f.Requests)
	}
}

func TestFetchChapters_AJAXWithoutMangaID(t *testing.T) {
	u := mustURL("https://site.org/manga/x/")
	page := `<div class="summary_image"></div><div id="manga-chapters-holder"></div>`
	f := &fetch.Static{Pages: map[string][]byte{u.String(): []byte(page)}}
	if _, err := New(clock).FetchChapters(context.Background(), f, u); !errors.Is(err, extract.ErrIncomplete) {
		t.Errorf("err = %v, want ErrIncomplete", err)
	}
}

func TestFetchChapters_DatesAndNumbers(t *testing.T) {
	u := mustURL("https://site.org/manga/x/")
	page := `<div class="summary_image"></div><ul>
	<li class="wp-manga-chapter"><a href="/manga/x/chapter-12/">Chapter 12 - The End</a>
	  <span class="chapter-release-date"><a href="#" title="2 days ago" class="c-new-tag">NEW</a></span></li>
	<li class="wp-manga-chapter"><a href="/manga/x/chapter-11-5/">Side story</a>
	  <span class="chapter-release-date"><i>October 1, 2026</i></span></li>
	<li class="wp-manga-chapter"><a href="/manga/x/extra/">Extra</a></li></ul>`
	f := &fetch.Static{Pages: map[string][]byte{u.String(): []byte(page)}}
	got, err := New(clock).FetchChapters(context.Background(), f, u)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		number *float64
		date   string
	}{{ptr(12), "2026-10-18"}, {ptr(11.5), "2026-10-01"}, {nil, ""}}
	for i, w := range want {
		g := got[i]
		if (g.Number == nil) != (w.number == nil) || (g.Number != nil && *g.Number != *w.number) || g.ReleasedAt.String() != w.date {
			t.Errorf("chapter %d = %+v (number %v), want %v %q", i, g, g.Number, w.number, w.date)
		}
	}
}

func TestAdapter_Match(t *testing.T) {
	a := New(nil)
	for _, tt := range []struct {
		url  string
		want bool
	}{
		{"https://www.mangaread.org/manga/eleceed/", true},
		{"https://www.mangaread.org/manga/eleceed/chapter-423/", true}, // read as its series
		{"https://www.mangaread.org/manga/eleceed/chapter-1/extra/", false},
		{"https://www.mangaread.org/genres/action/", false},
		{"https://other.org/manga/x/", false},
	} {
		if got := a.Match(mustURL(tt.url), nil); got != tt.want {
			t.Errorf("Match(%s, nil) = %v, want %v", tt.url, got, tt.want)
		}
	}
	u := mustURL("https://mirror.example/manga/x/")
	for html, want := range map[string]bool{
		`<div class="summary_image"></div><li class="wp-manga-chapter"></li>`:         true,
		`<div class="post-content_item"></div><div id="manga-chapters-holder"></div>`: true,
		`<li class="wp-manga-chapter"></li>`:                                          false, // no info box
		`<div class="summary_image"></div>`:                                           false, // no chapter list
	} {
		if got := a.Match(u, mustDoc(t, html)); got != want {
			t.Errorf("Match(%s) = %v, want %v", html, got, want)
		}
	}
	if !New(nil, "mirror.example").Match(u, nil) {
		t.Error("custom host should match")
	}
}

func TestFetchSeries_BadgesAndMissingTitle(t *testing.T) {
	u := mustURL("https://site.org/manga/x/")
	page := `<div class="post-title"><h1><span class="manga-title-badges hot">HOT</span> Solo</h1></div>
	<div class="post-content_item"><div class="summary-heading"><h5>Alternative</h5></div><div class="summary-content">Updating</div></div>
	<meta property="og:image" content="https://site.org/og.jpg">`
	f := &fetch.Static{Pages: map[string][]byte{u.String(): []byte(page)}}
	s, err := New(nil).FetchSeries(context.Background(), f, u)
	if err != nil || s.Title != "Solo" || len(s.AltTitles) != 0 || s.CoverURL != "https://site.org/og.jpg" || s.Type != domain.TypeOther {
		t.Errorf("series = %+v, %v", s, err)
	}
	f.Pages[u.String()] = []byte(`<div class="summary_image"></div>`)
	if _, err := New(nil).FetchSeries(context.Background(), f, u); !errors.Is(err, extract.ErrIncomplete) {
		t.Errorf("err = %v, want ErrIncomplete", err)
	}
}

func TestAdapter_ChapterLinkReadsTheSeries(t *testing.T) {
	const series = "https://www.mangaread.org/manga/eleceed/"
	f := &fetch.Static{Pages: map[string][]byte{series: read(t, "eleceed.html")}}
	res, err := extract.New(f, extract.Options{Adapters: []extract.Adapter{New(clock)}}).
		Extract(context.Background(), series+"chapter-1/?style=list")
	if err != nil {
		t.Fatal(err)
	}
	if res.Series.URL != series || len(res.Chapters) != 423 {
		t.Errorf("url %q, %d chapters", res.Series.URL, len(res.Chapters))
	}
}

func ptr(f float64) *float64 { return &f }

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
	u := mustURL("https://site.org/manga/x/")
	page := `<div class="summary_image"></div><ul>
	<li class="wp-manga-chapter"><a href="/manga/x/chapter-2/">Chapter 2</a></li>
	<li class="wp-manga-chapter"><a>Chapter 1</a></li></ul>`
	chs, err := New(clock).FetchChapters(context.Background(), &fetch.Static{Pages: map[string][]byte{u.String(): []byte(page)}}, u)
	if err != nil || len(chs) != 1 || *chs[0].Number != 2 {
		t.Errorf("chapters = %+v, %v", chs, err)
	}
}
