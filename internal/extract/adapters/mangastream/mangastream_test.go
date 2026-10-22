package mangastream

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

const fixtureDir = "../../../../testdata/fixtures/ravenscans.org"

func TestAdapter_Fixtures(t *testing.T) {
	tests := []struct {
		fixture    string
		url        string
		title      string
		cover      string
		status     domain.Status
		typ        domain.SeriesType
		altTitles  int
		chapters   int
		maxChapter float64
		latestDate string
	}{
		{
			fixture: "the-extras-academy-survival-guide",
			url:     "https://ravenscans.org/manga/the-extras-academy-survival-guide/",
			title:   "The Extra’s Academy Survival Guide",
			cover:   "https://ravenscans.org/wp-content/uploads/2025/05/the-extras-academy-survival-guide.jpg",
			status:  domain.StatusOngoing, typ: domain.TypeManhwa, altTitles: 3,
			chapters: 130, maxChapter: 128, latestDate: "2026-10-15",
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
			if res.Method != "adapter:mangastream" || s.Title != tt.title || s.CoverURL != tt.cover ||
				s.Status != tt.status || s.Type != tt.typ || len(s.AltTitles) != tt.altTitles || s.Synopsis == "" {
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
		{"https://ravenscans.org/manga/the-extras-academy-survival-guide/", true},
		{"https://ravenscans.org/the-extras-academy-survival-guide-chapter-128/", false},
		{"https://other.org/manga/x/", false},
	}
	for _, tt := range urlOnly {
		if got := a.Match(mustURL(tt.url), nil); got != tt.want {
			t.Errorf("Match(%s, nil) = %v, want %v", tt.url, got, tt.want)
		}
	}
	u := mustURL("https://mirror.example/manga/x/")
	theme := `<div class="imptdt">Status <i>Ongoing</i></div><div id="chapterlist"><ul><li></li></ul></div>`
	if !a.Match(u, mustDoc(t, theme)) {
		t.Error("theme markup should match")
	}
	if a.Match(u, mustDoc(t, `<div id="chapterlist"></div>`)) {
		t.Error("chapter list alone should not match")
	}
	if !New("mirror.example").Match(u, nil) {
		t.Error("custom host should match")
	}
}

func TestFetchChapters_FallbacksForNumberAndLabel(t *testing.T) {
	u := mustURL("https://ravenscans.org/manga/x/")
	html := `<div id="chapterlist"><ul>
		<li data-num="bad"><a href="/x-chapter-7/"><span class="chapternum">Chapter 7</span><span class="chapterdate">2 days ago</span></a></li>
		<li><a href="/x-chapter-6-5/">Ch. 6.5</a></li>
		<li><a href="/x-chapter-6/">Special</a></li>
	</ul></div>`
	f := &fetch.Static{Pages: map[string][]byte{u.String(): []byte(html)}}
	got, err := New().FetchChapters(context.Background(), f, u)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		number float64
		label  string
	}{{7, "Chapter 7"}, {6.5, "Ch. 6.5"}, {6, "Special"}}
	for i, w := range want {
		if got[i].Number == nil || *got[i].Number != w.number || got[i].Label != w.label {
			t.Errorf("chapter %d = %+v, want %v %q", i, got[i], w.number, w.label)
		}
	}
	if !got[0].ReleasedAt.IsZero() {
		t.Error("relative dates must stay unknown")
	}
}

func TestAdapter_MissingTitleIsIncomplete(t *testing.T) {
	u := mustURL("https://ravenscans.org/manga/x/")
	f := &fetch.Static{Pages: map[string][]byte{u.String(): []byte(`<div class="imptdt"></div>`)}}
	if _, err := New().FetchSeries(context.Background(), f, u); err == nil || !strings.Contains(err.Error(), "required field") {
		t.Errorf("err = %v, want ErrIncomplete", err)
	}
}

func TestCover_FallsBackToOGImage(t *testing.T) {
	d := mustDoc(t, `<meta property="og:image" content="https://a.org/og.jpg"><div class="thumb"><img src="data:image/gif;base64,x"></div>`)
	if got := cover(d); got != "https://a.org/og.jpg" {
		t.Errorf("cover = %q", got)
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
	u, _ := url.Parse("https://site.org/manga/x/")
	html := `<div id="chapterlist"><ul>
		<li><a href="https://site.org/x-chapter-2/"><span class="chapternum">Chapter 2</span></a></li>
		<li><span class="chapternum">Chapter 1</span></li></ul></div>`
	chs, err := New().FetchChapters(context.Background(), &fetch.Static{Pages: map[string][]byte{u.String(): []byte(html)}}, u)
	if err != nil || len(chs) != 1 || *chs[0].Number != 2 {
		t.Errorf("chapters = %+v, %v", chs, err)
	}
}
