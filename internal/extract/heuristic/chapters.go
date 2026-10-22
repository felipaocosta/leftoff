package heuristic

import (
	"cmp"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
)

// buttonTextRe recognizes call-to-action links ("Read (20)", "Start
// reading", "Ler agora") whose text is not a chapter label.
var buttonTextRe = regexp.MustCompile(`(?i)^(read|start|continue|begin|first|latest|newest|ler|leer|come[cç]ar|continuar)\b`)

// chapterHintRe recognizes chapter links by text or href (SPEC §5.4).
var chapterHintRe = regexp.MustCompile(`(?i)\b(chapter|chap|ch|cap[ií]tulo|cap|episode|ep)[\s.\-_:#]*\d`)

// findChapters collects chapter-like links and keeps the largest group that
// shares a URL prefix (the directory of the link path), which separates the
// real chapter list from "latest chapter" links of other series. Links are
// returned in page order, without repeated URLs.
func findChapters(doc *goquery.Document, base *url.URL, now time.Time) []domain.ChapterInfo {
	groups := map[string][]domain.ChapterInfo{}
	var order []string
	seen := map[string]int{} // href → index in its group

	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href := extract.ResolveURL(base, a.AttrOr("href", ""))
		text := linkLabel(a)
		if href == "" || !looksLikeChapter(text, href) {
			return
		}
		key := groupKey(href)
		if i, dup := seen[href]; dup {
			// A "Read now" button may come before the list entry of the same
			// chapter: keep the first position but the more descriptive label.
			if c := &groups[key][i]; !chapterHintRe.MatchString(c.Label) && chapterHintRe.MatchString(text) {
				c.Label = text
			}
			return
		}
		number := extract.ParseChapterNumber(text)
		if number == nil {
			number = extract.ParseChapterNumber(href)
		}
		if number != nil && buttonTextRe.MatchString(text) {
			text = "" // a button such as "Read (20)": extract labels it "Chapter N"
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		seen[href] = len(groups[key])
		groups[key] = append(groups[key], domain.ChapterInfo{
			Number: number, Label: text, URL: href, ReleasedAt: linkDate(a, now),
		})
	})

	var best []domain.ChapterInfo
	for _, k := range order { // first group wins ties: page order is stable
		if len(groups[k]) > len(best) {
			best = groups[k]
		}
	}
	sortNewestFirst(best)
	return best
}

// sortNewestFirst orders chapters by number, highest first, like the
// adapters do; chapters without a number keep their page order at the end.
// Page order alone is unreliable ("read first chapter" buttons, paginated
// lists copied out of order).
func sortNewestFirst(chs []domain.ChapterInfo) {
	slices.SortStableFunc(chs, func(a, b domain.ChapterInfo) int {
		switch {
		case a.Number == nil && b.Number == nil:
			return 0
		case a.Number == nil:
			return 1
		case b.Number == nil:
			return -1
		}
		return cmp.Compare(*b.Number, *a.Number)
	})
}

// linkDate finds a release date inside or right after a chapter link: a
// <time datetime>, a text chunk ("8 days ago", "Oct 9, 2026") or a title
// attribute ("about 11 hours"). The link's parent row is searched too, since
// many lists put the date next to the link.
func linkDate(a *goquery.Selection, now time.Time) domain.Date {
	scope := a
	for range 3 {
		if d, ok := dateIn(scope, now); ok {
			return d
		}
		// Climb only while the row holds just this link; higher up is the
		// whole list, whose first date would be copied to every chapter.
		scope = scope.Parent()
		if scope.Length() == 0 || scope.Find("a[href]").Length() > 1 {
			break
		}
	}
	return domain.Date{}
}

func dateIn(scope *goquery.Selection, now time.Time) (domain.Date, bool) {
	if dt, ok := scope.Find("time[datetime]").First().Attr("datetime"); ok {
		if t, err := time.Parse(time.RFC3339, dt); err == nil {
			t = t.UTC()
			return domain.NewDate(t.Year(), t.Month(), t.Day()), true
		}
	}
	for _, c := range extract.TextChunks(scope) {
		if chapterHintRe.MatchString(c) {
			continue // "Chapter 14" is a label, not a date
		}
		if d, ok := extract.ParseReleaseDate(c, now); ok {
			return d, true
		}
	}
	var found domain.Date
	ok := false
	scope.Find("[title]").EachWithBreak(func(_ int, el *goquery.Selection) bool {
		found, ok = extract.ParseReleaseDate(el.AttrOr("title", ""), now)
		return !ok
	})
	return found, ok
}

// linkLabel is the chapter label of a link: the first text chunk that looks
// like a chapter ("Chapter 14", not the "8 days ago" or "New" next to it),
// else all its text, else the alt text of an image inside it.
func linkLabel(a *goquery.Selection) string {
	chunks := extract.TextChunks(a)
	for _, c := range chunks {
		if chapterHintRe.MatchString(c) {
			return c
		}
	}
	if len(chunks) > 0 {
		return strings.Join(chunks, " ")
	}
	return extract.CleanText(a.Find("img[alt]").First().AttrOr("alt", ""))
}

func looksLikeChapter(text, href string) bool {
	return chapterHintRe.MatchString(text) || chapterHintRe.MatchString(href)
}

// groupKey is host + parent directory of the link path:
// https://a.org/manga/x/chapter-1/ → a.org/manga/x.
func groupKey(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	return strings.ToLower(u.Host) + path.Dir(strings.TrimSuffix(u.Path, "/"))
}
