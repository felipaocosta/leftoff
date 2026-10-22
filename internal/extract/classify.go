package extract

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Classification of chapter pages and lookup of their series page (RF-03,
// SPEC §5.1 step 1). All patterns live here (RULES §6.6).
var (
	// chapterPathRe matches URL paths that address one chapter.
	chapterPathRe = regexp.MustCompile(`(?i)(chapter|chap|episode|capitulo|cap[ií]tulo)[-_/]?\d|/ch[-_.]?\d|/(read|reader|chapter|leer|ler)/`)
	// prevNextRe matches the reading navigation of chapter pages.
	prevNextRe = regexp.MustCompile(`(?i)^\s*(«|‹|<)?\s*(prev(ious)?|next|anterior|pr[oó]ximo|siguiente)\b`)
	// indexTextRe matches links that point to the series index.
	// Generic words such as "Manga" or "Series" are left out: they usually
	// point to the site-wide listing, not to this series.
	indexTextRe = regexp.MustCompile(`(?i)^\s*(index|table of contents|toc|all chapters|chapter list|chapters|[ií]ndice|cap[ií]tulos|lista de cap[ií]tulos|todos os cap[ií]tulos)\s*$`)
	// namePrefixRe drops site prefixes from titles ("Manga: Grand Finale").
	namePrefixRe = regexp.MustCompile(`(?i)^(read|manga|manhwa|manhua|novel|comic)\s*:?\s+`)
	// titleChapterRe splits "Series Name Chapter 12" into the series name.
	titleChapterRe = regexp.MustCompile(`(?i)^(.+?)\s*[-–—:|]*\s*\b(chapter|chap|ch\.?|episode|ep\.?|cap[ií]tulo|cap\.?)\s*\d`)
)

const (
	breadcrumbSel = `.breadcrumb a, .breadcrumbs a, nav[aria-label*="readcrumb"] a, [itemtype*="BreadcrumbList"] a`
	// seriesListMin chapter links mean the page itself is a series page.
	seriesListMin = 10
	maxCandidates = 5
)

// isChapterPage scores chapter signals: a chapter URL, a chapter title and
// previous/next links. A page that lists many chapters is a series page.
func isChapterPage(u *url.URL, doc *goquery.Document) bool {
	if chapterLinkCount(doc, u) >= seriesListMin {
		return false
	}
	score := 0
	if chapterPathRe.MatchString(u.Path) {
		score++
	}
	if ParseChapterNumber(doc.Find("h1").First().Text()) != nil || ParseChapterNumber(doc.Find("title").Text()) != nil {
		score++
	}
	if hasPrevNext(doc) {
		score++
	}
	return score >= 2
}

// ChapterLinkCount counts the distinct links whose text reads as a
// chapter ("Chapter 12"); a JavaScript-only page has few or none.
func ChapterLinkCount(doc *goquery.Document, base *url.URL) int {
	return chapterLinkCount(doc, base)
}

func chapterLinkCount(doc *goquery.Document, base *url.URL) int {
	seen := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := ResolveURL(base, a.AttrOr("href", ""))
		if href != "" && ParseChapterNumber(CleanText(a.Text())) != nil {
			seen[href] = true
		}
	})
	return len(seen)
}

func hasPrevNext(doc *goquery.Document) bool {
	if doc.Find(`a[rel="next"], a[rel="prev"]`).Length() > 0 {
		return true
	}
	found := false
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		found = prevNextRe.MatchString(CleanText(a.Text()))
		return !found
	})
	return found
}

// seriesCandidates lists likely series pages for a chapter page, best
// first: index links, breadcrumbs, links named after the series, the URL
// without its chapter segment, and finally the site's home page (single-
// series sites). Only same-site links are considered.
func seriesCandidates(u *url.URL, doc *goquery.Document) []string {
	var out []string
	seen := map[string]bool{CanonicalKey(u): true}
	add := func(raw string) {
		c, err := url.Parse(raw)
		if raw == "" || err != nil || !sameSite(u, c) || seen[CanonicalKey(c)] || len(out) >= maxCandidates {
			return
		}
		seen[CanonicalKey(c)] = true
		out = append(out, c.String())
	}
	links := func(match func(text string) bool) {
		doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
			if match(CleanText(a.Text())) {
				add(ResolveURL(u, a.AttrOr("href", "")))
			}
		})
	}
	links(indexTextRe.MatchString)
	crumbs := doc.Find(breadcrumbSel)
	for i := crumbs.Length() - 1; i >= 0; i-- { // deepest crumb first
		add(ResolveURL(u, crumbs.Eq(i).AttrOr("href", "")))
	}
	for _, name := range seriesNames(doc) {
		links(func(text string) bool { return strings.EqualFold(text, name) })
	}
	add(parentURL(u))
	add(u.Scheme + "://" + u.Host + "/")
	return out
}

// seriesNames guesses the series name of a chapter page: the part before
// "Chapter N" in the <h1> or <title> ("Eternally Regressing Knight Chapter
// 112.7" → "Eternally Regressing Knight"), or the <h1> itself when it has no
// chapter number (readers often title the page with the series name).
func seriesNames(doc *goquery.Document) []string {
	var names []string
	h1 := CleanText(doc.Find("h1").First().Text())
	for _, t := range []string{h1, CleanText(doc.Find("title").Text())} {
		if m := titleChapterRe.FindStringSubmatch(t); m != nil {
			names = append(names, strings.TrimSpace(namePrefixRe.ReplaceAllString(m[1], "")))
		}
	}
	if h1 != "" && ParseChapterNumber(h1) == nil {
		names = append(names, h1)
	}
	return names
}

// parentURL drops the chapter segment: /manga/x/chapter-5/ → /manga/x/.
func parentURL(u *url.URL) string {
	p := strings.TrimSuffix(u.Path, "/")
	if !chapterPathRe.MatchString(path.Base(p)) && !chapterPathRe.MatchString(p) {
		return ""
	}
	dir := path.Dir(p)
	if dir == "/" || dir == "." {
		return ""
	}
	return u.Scheme + "://" + u.Host + dir + "/"
}

// sameSite reports whether b is on the same site as a ("www." and other
// subdomains of the same domain count, e.g. w12.site.com and site.com).
func sameSite(a, b *url.URL) bool {
	return registrable(a.Hostname()) == registrable(b.Hostname())
}

// registrable approximates the registrable domain by its last two labels.
func registrable(host string) string {
	labels := strings.Split(strings.ToLower(host), ".")
	if len(labels) <= 2 {
		return strings.Join(labels, ".")
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

// containsChapter reports whether res lists the chapter at chapterURL
// (ignoring scheme, "www." and trailing slashes), which confirms that a
// candidate is the series of that chapter.
func containsChapter(res *Result, chapterURL *url.URL) bool {
	want := CanonicalKey(chapterURL)
	for _, c := range res.Chapters {
		if cu, err := url.Parse(c.URL); err == nil && CanonicalKey(cu) == want {
			return true
		}
		for _, alt := range c.AltURLs {
			if au, err := url.Parse(alt); err == nil && CanonicalKey(au) == want {
				return true
			}
		}
	}
	return false
}
