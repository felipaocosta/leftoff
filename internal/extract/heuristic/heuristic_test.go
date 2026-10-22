package heuristic

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

const base = "https://site.org/manga/solo/"

var testNow = time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)

func page(t *testing.T, html string) *extract.Page {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	return &extract.Page{URL: u, Doc: doc}
}

func doc(t *testing.T, html string) *goquery.Document { return page(t, html).Doc }

func TestFindTitle(t *testing.T) {
	tests := []struct {
		name, html, want string
	}{
		{"json-ld wins", `<script type="application/ld+json">{"@type":"Book","name":"LD Title"}</script>
			<meta property="og:title" content="OG"><h1>H1</h1>`, "LD Title"},
		{"json-ld inside @graph", `<script type="application/ld+json">{"@graph":[{"@type":"WebPage","name":"Page"},
			{"@type":["Thing","ComicSeries"],"name":"Graph Title"}]}</script>`, "Graph Title"},
		{"og cleaned", `<meta property="og:title" content="Read Solo Leveling Manga Online - Site Scans">`, "Solo Leveling"},
		{"h1 preferred when og contains it", `<meta property="og:title" content="Solo Leveling | Site"><h1> Solo  Leveling </h1>`, "Solo Leveling"},
		{"og when h1 is unrelated", `<meta property="og:title" content="Solo Leveling"><h1>Site Name</h1>`, "Solo Leveling"},
		{"h1 only", `<h1>Only H1</h1>`, "Only H1"},
		{"og is only the site name", `<meta property="og:title" content="Site - Read Manga Online Free"><h1>Real Title</h1>`, "Real Title"},
		{"non-work json-ld ignored", `<script type="application/ld+json">[{"@type":"WebSite","name":"Site"}]</script><h1>Real</h1>`, "Real"},
		{"broken json-ld ignored", `<script type="application/ld+json">{oops</script><h1>Real</h1>`, "Real"},
		{"nothing", `<p>no title</p>`, ""},
		{"og with type and decoration", `<meta property="og:title" content="Healing Store Manhwa | Read Online Free at site.org">`, "Healing Store"},
		{"og type word kept without decoration", `<meta property="og:title" content="I Became a Manga - Volume Two">`, "I Became a Manga"},
		{"decorated json-ld without h1", `<script type="application/ld+json">{"@type":"Book",
			"name":"Healing Store Manhwa | Read Online Free at site.org"}</script>`, "Healing Store"},
		{"json-ld with the site name", `<script type="application/ld+json">{"@type":"Book","name":"Solo Leveling - Site"}</script>`, "Solo Leveling"},
		{"json-ld subtitle kept", `<script type="application/ld+json">{"@type":"Book",
			"name":"Re:Zero - Starting Life in Another World"}</script>`, "Re:Zero - Starting Life in Another World"},
		{"plain title ending in a type word", `<script type="application/ld+json">{"@type":"Book","name":"I Became a Manga"}</script>`, "I Became a Manga"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := doc(t, tt.html)
			if got := findTitle(d, findJSONLD(d), mustURL(base)); got != tt.want {
				t.Errorf("findTitle = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindCover(t *testing.T) {
	tests := []struct {
		name, html, want string
	}{
		{"json-ld image object", `<script type="application/ld+json">{"@type":"Book","name":"x","image":{"url":"https://c.org/ld.jpg"}}</script>`, "https://c.org/ld.jpg"},
		{"json-ld image list", `<script type="application/ld+json">{"@type":"Book","name":"x","image":["https://c.org/1.jpg"]}</script>`, "https://c.org/1.jpg"},
		{"og image", `<meta property="og:image" content="https://c.org/og.jpg">`, "https://c.org/og.jpg"},
		{"marked cover beats og banner", `<meta property="og:image" content="https://site.org/api/og-image/x.webp">
			<img itemprop="image" alt="Cover of X" src="/covers/x.png">`, "https://site.org/covers/x.png"},
		{"alt names the cover", `<meta property="og:image" content="https://c.org/og.jpg"><img alt="Capa de X" data-src="/c.jpg" src="data:,">`, "https://site.org/c.jpg"},
		{"landscape og falls back to image near title", `<meta property="og:image" content="https://c.org/wide.jpg">
			<meta property="og:image:width" content="1200"><meta property="og:image:height" content="630">
			<div><img src="/poster.jpg" width="300" height="450"><h1>T</h1></div>`, "https://site.org/poster.jpg"},
		{"file named cover", `<img src="/logo.png"><img src="/up/Some-Series-Cover.webp"><h1>T</h1>`, "https://site.org/up/Some-Series-Cover.webp"},
		{"og wins over a file named cover", `<meta property="og:image" content="https://c.org/og.jpg"><img src="/related-cover.jpg">`, "https://c.org/og.jpg"},
		{"banner kept when nothing else", `<meta property="og:image" content="https://c.org/og-image/x.png"><h1>T</h1>`, "https://c.org/og-image/x.png"},
		{"largest image near title, lazy src, skips logo", `<div><div class="info">
			<img src="/logo.png" width="900" height="900">
			<img src="/small.jpg" width="50" height="50">
			<img data-src="/cover.jpg" src="data:image/gif;base64,x" width="300" height="400">
			<h1>T</h1></div></div>`, "https://site.org/cover.jpg"},
		{"no image", `<h1>T</h1>`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := page(t, tt.html)
			if got := findCover(p.Doc, findJSONLD(p.Doc), p.URL); got != tt.want {
				t.Errorf("findCover = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindStatusText(t *testing.T) {
	tests := []struct {
		name, html, want string
	}{
		{"label in parent", `<div class="imptdt">Status <i>Ongoing</i></div>`, "Ongoing"},
		{"label with colon", `<li><b>Status:</b> Completed</li>`, "Completed"},
		{"definition list", `<dl><dt>Status</dt><dd>Em andamento</dd></dl>`, "Em andamento"},
		{"portuguese label", `<p><span>Situação:</span> Finalizado</p>`, "Finalizado"},
		{"long paragraph is not a label", `<p>Status of this world is a long story that goes on and on and on.</p>`, ""},
		{"none", `<p>nothing</p>`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findStatusText(doc(t, tt.html)); got != tt.want {
				t.Errorf("findStatusText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindChapters_ReadsReleaseDates(t *testing.T) {
	html := `<ul>
		<li><a href="/manga/solo/chapter-4"><span>Chapter<!-- --> 4</span><span title="about 11 hours"></span></a></li>
		<li><a href="/manga/solo/chapter-3"><span>Chapter 3</span><span>8 days</span></a></li>
		<li><a href="/manga/solo/chapter-2">Chapter 2</a><span class="date">Oct 9, 2026</span></li>
		<li><a href="/manga/solo/chapter-1">Chapter 1</a><time datetime="2026-09-01T10:00:00Z">long ago</time></li>
		<li><a href="/manga/solo/chapter-0">Chapter 0</a> <span>about 1 month</span></li>
	</ul>`
	got := findChapters(doc(t, html), mustURL(base), testNow)
	want := []string{"2026-10-19", "2026-10-12", "2026-10-09", "2026-09-01", ""}
	for i, w := range want {
		if got[i].ReleasedAt.String() != w {
			t.Errorf("chapter %v date = %q, want %q", *got[i].Number, got[i].ReleasedAt, w)
		}
	}
}

func TestFindChapters_DoesNotShareDatesAcrossTheList(t *testing.T) {
	html := `<div><span>3 days ago</span><a href="/manga/solo/chapter-2">Chapter 2</a><a href="/manga/solo/chapter-1">Chapter 1</a></div>`
	for _, c := range findChapters(doc(t, html), mustURL(base), testNow) {
		if !c.ReleasedAt.IsZero() {
			t.Errorf("chapter %v got a date from a shared container: %s", *c.Number, c.ReleasedAt)
		}
	}
}

func TestSortNewestFirst_KeepsNumberlessAtTheEndInOrder(t *testing.T) {
	n := func(v float64) *float64 { return &v }
	chs := []domain.ChapterInfo{{Number: n(1)}, {Label: "Extra A"}, {Number: n(20)}, {Label: "Extra B"}, {Number: n(10.5)}}
	sortNewestFirst(chs)
	got := []string{}
	for _, c := range chs {
		if c.Number != nil {
			got = append(got, domain.ChapterKey(c.Number, ""))
		} else {
			got = append(got, c.Label)
		}
	}
	if strings.Join(got, ",") != "20,10.5,1,Extra A,Extra B" {
		t.Errorf("order = %v", got)
	}
}

func TestBadges_StatusAndTypeNearTitle(t *testing.T) {
	html := `<nav><a>Completed</a><a>Manhwa</a></nav>
		<section><div><h1>T</h1><div><span>Manga</span><span><i class="dot"></i>Releasing</span><span>2022</span></div></div></section>`
	p := page(t, html)
	if got := findStatusText(p.Doc); got != "Releasing" {
		t.Errorf("status = %q, want the badge near the title, not the menu", got)
	}
	if got := guessType(findBadge(p.Doc, isTypeBadge)); got != domain.TypeManga {
		t.Errorf("type = %q", got)
	}
	if isTypeBadge("Read Manga") {
		t.Error("type badges must match exactly")
	}
}

func TestFindChapters_KeepsLargestGroupInPageOrder(t *testing.T) {
	html := `
		<div class="sidebar">
			<a href="/manga/other/chapter-99/">Chapter 99</a>
			<a href="/manga/another/chapter-5/">Chapter 5</a>
		</div>
		<ul class="list">
			<li><a href="/manga/solo/chapter-3/">Chapter 3</a></li>
			<li><a href="/manga/solo/chapter-2-5/">Bonus</a></li>
			<li><a href="/manga/solo/chapter-2/">Chapter 2</a></li>
			<li><a href="/manga/solo/chapter-2/">Chapter 2 (dup link)</a></li>
			<li><a href="/manga/solo/ep-1/">Ep. 1</a></li>
		</ul>
		<a href="/about/">About</a>
		<a href="javascript:void(0)">Chapter 1</a>`
	got := findChapters(doc(t, html), mustURL(base), testNow)
	want := []struct {
		number float64
		label  string
	}{{3, "Chapter 3"}, {2.5, "Bonus"}, {2, "Chapter 2"}, {1, "Ep. 1"}} // newest first
	if len(got) != len(want) {
		t.Fatalf("got %d chapters: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].Number == nil || *got[i].Number != w.number || got[i].Label != w.label {
			t.Errorf("chapter %d = %+v, want %v %q", i, got[i], w.number, w.label)
		}
	}
}

func TestFindChapters_PrefersChapterLabelOverButtonText(t *testing.T) {
	html := `<a href="/manga/solo/chapter-0">Read Now</a>
		<ul><li><a href="/manga/solo/chapter-1">Chapter 1</a></li>
		<li><a href="/manga/solo/chapter-0">Chapter 0</a></li></ul>`
	got := findChapters(doc(t, html), mustURL(base), testNow)
	// Sorted newest first: chapter 1, then chapter 0 with the list label.
	if len(got) != 2 || *got[1].Number != 0 || got[1].Label != "Chapter 0" {
		t.Errorf("chapters = %+v", got)
	}
}

func TestExtract_FullPageThroughPipeline(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="Solo Leveling - Site">
		<meta property="og:image" content="/cover.jpg">
		<meta name="description" content="  A  hunter. ">
		</head><body><h1>Solo Leveling</h1>
		<div>Status <span>OnGoing</span></div>
		<a href="chapter-3/">Chapter 3</a><a href="chapter-2/">Chapter 2</a><a href="chapter-1/">Chapter 1</a>
		</body></html>`
	f := &fetch.Static{Pages: map[string][]byte{base: []byte(html)}}
	res, err := extract.New(f, extract.Options{Fallback: New()}).Extract(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Series
	if res.Method != Method || s.Title != "Solo Leveling" || s.CoverURL != "https://site.org/cover.jpg" ||
		s.Status != domain.StatusOngoing || s.Synopsis != "A hunter." || len(res.Chapters) != 3 {
		t.Errorf("result = %+v", res)
	}
	if res.Confidence != 1 {
		t.Errorf("confidence = %v, want 1", res.Confidence)
	}
}

func TestExtract_FewChaptersNeedConfirmation(t *testing.T) {
	html := `<html><head><meta property="og:image" content="/cover.jpg"></head><body><h1>Solo Leveling</h1>
		<div>Status <span>OnGoing</span></div><a href="chapter-1/">Chapter 1</a></body></html>`
	f := &fetch.Static{Pages: map[string][]byte{base: []byte(html)}}
	res, err := extract.New(f, extract.Options{Fallback: New()}).Extract(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if res.Confidence >= extract.LowConfidence {
		t.Errorf("confidence = %v with %d chapter(s): the preview would not ask", res.Confidence, len(res.Chapters))
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func TestExtract_SiteNameWithoutChaptersIsNoTitle(t *testing.T) {
	const shell = `<html><head><meta property="og:title" content="Site"></head><body><div id="app"></div></body></html>`
	const single = `<html><head><meta property="og:title" content="Site"></head><body><h1>Site</h1>
		<a href="/chapter-3/">Chapter 3</a><a href="/chapter-2/">Chapter 2</a><a href="/chapter-1/">Chapter 1</a></body></html>`
	for html, want := range map[string]string{shell: "", single: "Site"} {
		f := &fetch.Static{Pages: map[string][]byte{base: []byte(html)}}
		res, err := extract.New(f, extract.Options{Fallback: New()}).Extract(context.Background(), base)
		switch {
		case want == "" && !errors.Is(err, extract.ErrIncomplete):
			t.Errorf("JavaScript shell: err = %v, want ErrIncomplete", err)
		case want != "" && (err != nil || res.Series.Title != want):
			t.Errorf("single-series site: %+v, %v", res, err)
		}
	}
}
