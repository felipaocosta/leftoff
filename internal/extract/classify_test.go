package extract

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

func docOf(t *testing.T, html string) *goquery.Document {
	t.Helper()
	d, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func mustParse(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestIsChapterPage(t *testing.T) {
	list := ""
	for i := range 12 {
		list += `<a href="/manga/x/chapter-` + string(rune('a'+i)) + `">Chapter ` + string(rune('1'+i%9)) + `</a>`
	}
	tests := []struct {
		name, url, html string
		want            bool
	}{
		{"chapter url and title", "https://a.org/manga/x/chapter-5/", `<h1>X Chapter 5</h1>`, true},
		{"title and prev/next (no chapter url)", "https://a.org/season-1-afterword/", `<h1>X Chapter 112.7</h1><a href="/p">« Previous</a>`, true},
		{"rel=next and chapter url", "https://a.org/read/x/ch1-2", `<a rel="next" href="/n">›</a>`, true},
		{"series page with a long list", "https://a.org/manga/x/chapter-5/", `<h1>X Chapter 5</h1>` + list, false},
		{"plain series page", "https://a.org/manga/x/", `<h1>X</h1>`, false},
		{"one signal is not enough", "https://a.org/manga/x/", `<h1>X</h1><a href="/n">Next</a>`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isChapterPage(mustParse(tt.url), docOf(t, tt.html)); got != tt.want {
				t.Errorf("isChapterPage = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeriesCandidates_Order(t *testing.T) {
	u := mustParse("https://w12.site.com/manga/my-series/chapter-5/")
	html := `<title>My Series Chapter 5 – Site</title>
		<a href="/manga/">Manga</a>
		<a href="https://other.org/manga/x/">Index</a>
		<nav class="breadcrumb"><a href="/">Home</a><a href="/manga/">Manga</a><a href="/manga/my-series/">My Series</a></nav>
		<a href="/toc/my-series">Table of Contents</a>
		<a href="/series/my-series-info">My Series</a>
		<a href="https://site.com/">Home</a>`
	got := seriesCandidates(u, docOf(t, html))
	want := []string{
		"https://w12.site.com/toc/my-series",    // index link (other.org is another site)
		"https://w12.site.com/manga/my-series/", // deepest breadcrumb
		"https://w12.site.com/manga/",
		"https://w12.site.com/",
		"https://w12.site.com/series/my-series-info", // link named after the series
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("candidates =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestHelpers_ParentNameSite(t *testing.T) {
	if got := parentURL(mustParse("https://a.org/manga/x/chapter-5/")); got != "https://a.org/manga/x/" {
		t.Errorf("parentURL = %q", got)
	}
	if got := parentURL(mustParse("https://a.org/x-chapter-5/")); got != "" {
		t.Errorf("parentURL at the root = %q, want none", got)
	}
	if got := seriesNames(docOf(t, `<h1>Eternally Regressing Knight Chapter 112.7</h1>`)); len(got) == 0 || got[0] != "Eternally Regressing Knight" {
		t.Errorf("seriesNames = %q", got)
	}
	if got := seriesNames(docOf(t, `<title>Manga: Grand Finale Chapter - 50-eng-li</title><h1>Grand Finale</h1>`)); strings.Join(got, "|") != "Grand Finale" {
		t.Errorf("seriesNames (reader page) = %q", got)
	}
	if !sameSite(mustParse("https://w12.regressingknight.com/"), mustParse("https://regressingknight.com/x")) ||
		sameSite(mustParse("https://a.org/"), mustParse("https://b.org/")) {
		t.Error("sameSite")
	}
}

// TestExtract_ChapterURLFindsTheSeries uses the real regressingknight pages:
// a chapter URL resolves to the home page, which lists that chapter.
func TestExtract_ChapterURLFindsTheSeries(t *testing.T) {
	read := func(n string) []byte {
		b, err := testfixture.ReadFile(t, "../../testdata/fixtures/regressingknight.com/"+n)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	const chapter = "https://w12.regressingknight.com/season-1-afterword-part-3/"
	f := &fetch.Static{Pages: map[string][]byte{
		chapter:                             read("season-1-afterword-part-3.html"),
		"https://w12.regressingknight.com/": read("home.html"),
	}}
	res, err := New(f, Options{Fallback: linksFallback{}}).Extract(context.Background(), chapter)
	if err != nil {
		t.Fatal(err)
	}
	if res.Series.URL != "https://w12.regressingknight.com/" || len(res.Chapters) < 100 {
		t.Errorf("series = %s with %d chapters", res.Series.URL, len(res.Chapters))
	}

	// The same chapter with a home page that does not list it is refused.
	f.Pages["https://w12.regressingknight.com/"] = []byte(`<h1>Another series</h1><a href="/x-chapter-1/">Chapter 1</a>`)
	_, err = New(f, Options{Fallback: linksFallback{}}).Extract(context.Background(), chapter)
	if !errors.Is(err, ErrChapterPage) {
		t.Errorf("err = %v, want ErrChapterPage", err)
	}
}

// linksFallback is a tiny heuristic: the <h1> as title and every link
// whose text has a chapter number as a chapter.
type linksFallback struct{}

func (linksFallback) Extract(_ context.Context, _ fetch.Fetcher, p *Page) (*Result, error) {
	res := &Result{Method: "heuristic", Series: domain.SeriesInfo{Title: p.Doc.Find("h1").First().Text()}}
	p.Doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		if n := ParseChapterNumber(a.Text()); n != nil {
			res.Chapters = append(res.Chapters, domain.ChapterInfo{Number: n, Label: CleanText(a.Text()), URL: ResolveURL(p.URL, a.AttrOr("href", ""))})
		}
	})
	return res, nil
}

func TestContainsChapter(t *testing.T) {
	res := &Result{Chapters: []domain.ChapterInfo{{URL: "https://site.org/x-chapter-2"}, {URL: "https://a.org/1", AltURLs: []string{"https://www.site.org/x-chapter-1/"}}}}
	for raw, want := range map[string]bool{
		"http://site.org/x-chapter-2/": true,
		"https://site.org/x-chapter-1": true,
		"https://site.org/x-chapter-3": false,
	} {
		if got := containsChapter(res, mustParse(raw)); got != want {
			t.Errorf("containsChapter(%s) = %v", raw, got)
		}
	}
}
