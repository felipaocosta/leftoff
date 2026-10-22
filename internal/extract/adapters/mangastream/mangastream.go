// Package mangastream is the native adapter for WordPress sites running the
// MangaStream / MangaReader themes, such as ravenscans.org (SPEC §14). The
// series page lists every chapter, so a single request serves both calls.
//
// It does NOT handle chapter pages (RF-03) nor chapter lists loaded by AJAX.
package mangastream

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Selectors and patterns (RULES §6.6).
const (
	selTitle         = "h1.entry-title"
	selAltTitles     = ".alternative"
	selCoverImg      = ".thumb img"
	selCoverMeta     = `meta[property="og:image"]`
	selInfoRow       = ".imptdt"
	selSynopsis      = `.entry-content[itemprop="description"]`
	selChapterList   = "#chapterlist"
	selChapterItem   = "#chapterlist li"
	selChapterLink   = "a[href]"
	selChapterLabel  = ".chapternum"
	selChapterDate   = ".chapterdate"
	attrChapterNum   = "data-num"
	statusLabel      = "status"
	typeLabel        = "type"
	maxInfoLabelSize = 30
)

var (
	// seriesPathRe matches series pages (/manga/<slug>/); chapters live at
	// /<slug>-chapter-N/ and do not match.
	seriesPathRe  = regexp.MustCompile(`^/manga/[^/]+/?$`)
	altTitleSepRe = regexp.MustCompile(`\s*[,•;|]\s*`)
	// dateLayouts are the release date formats seen on these themes.
	dateLayouts = []string{"January 2, 2006", "Jan 2, 2006", "2006-01-02"}
)

func defaultHosts() []string { return []string{"ravenscans.org"} }

// Adapter implements extract.Adapter.
type Adapter struct{ hosts map[string]bool }

// New returns the adapter. hosts lists sites matched by URL alone and
// defaults to the known sites (ravenscans.org).
func New(hosts ...string) *Adapter {
	if len(hosts) == 0 {
		hosts = defaultHosts()
	}
	a := &Adapter{hosts: map[string]bool{}}
	for _, h := range hosts {
		a.hosts[strings.ToLower(h)] = true
	}
	return a
}

// Name implements extract.Adapter.
func (*Adapter) Name() string { return "mangastream" }

// Match accepts series pages of known hosts by URL, or any page with the
// theme's chapter list and info rows.
func (a *Adapter) Match(u *url.URL, doc *goquery.Document) bool {
	if doc == nil {
		return a.hosts[extract.Domain(u)] && seriesPathRe.MatchString(u.Path)
	}
	return doc.Find(selChapterList).Length() > 0 && doc.Find(selInfoRow).Length() > 0
}

// FetchSeries implements extract.Adapter.
func (*Adapter) FetchSeries(ctx context.Context, f fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error) {
	doc, err := load(ctx, f, u)
	if err != nil {
		return nil, err
	}
	title := extract.CleanText(doc.Find(selTitle).First().Text())
	if title == "" {
		return nil, fmt.Errorf("mangastream %s: title: %w", u, extract.ErrIncomplete)
	}
	info := infoRows(doc)
	return &domain.SeriesInfo{
		URL:       u.String(),
		Title:     title,
		AltTitles: altTitles(doc),
		CoverURL:  cover(doc),
		StatusRaw: info[statusLabel],
		Synopsis:  extract.CleanText(doc.Find(selSynopsis).First().Text()),
		Type:      seriesType(info[typeLabel]),
	}, nil
}

// FetchChapters implements extract.Adapter. Chapters come newest first, as
// on the page.
func (*Adapter) FetchChapters(ctx context.Context, f fetch.Fetcher, u *url.URL) ([]domain.ChapterInfo, error) {
	doc, err := load(ctx, f, u)
	if err != nil {
		return nil, err
	}
	var chapters []domain.ChapterInfo
	doc.Find(selChapterItem).Each(func(_ int, li *goquery.Selection) {
		link := li.Find(selChapterLink).First()
		href := link.AttrOr("href", "")
		if href == "" {
			return // a broken item (cut-off page): no chapter to open
		}
		label := extract.CleanText(li.Find(selChapterLabel).First().Text())
		if label == "" {
			label = extract.CleanText(link.Text())
		}
		chapters = append(chapters, domain.ChapterInfo{
			Number:     chapterNumber(li, label, href),
			Label:      label,
			URL:        href,
			ReleasedAt: parseDate(li.Find(selChapterDate).First().Text()),
		})
	})
	return chapters, nil
}

func load(ctx context.Context, f fetch.Fetcher, u *url.URL) (*goquery.Document, error) {
	resp, err := fetch.Get(ctx, f, u.String())
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("mangastream: parse %s: %w", u, err)
	}
	return doc, nil
}

// chapterNumber prefers the data-num attribute, then the label, then the URL.
func chapterNumber(li *goquery.Selection, label, href string) *float64 {
	if raw, ok := li.Attr(attrChapterNum); ok {
		if n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
			return &n
		}
	}
	if n := extract.ParseChapterNumber(label); n != nil {
		return n
	}
	return extract.ParseChapterNumber(href)
}

// infoRows maps lowercase row labels ("status", "type") to their values.
// A row is "Label <i>value</i>" or "Label <a>value</a>": the label is the
// row's own text and the value is the text of its children.
func infoRows(doc *goquery.Document) map[string]string {
	rows := map[string]string{}
	doc.Find(selInfoRow).Each(func(_ int, row *goquery.Selection) {
		label := strings.ToLower(extract.CleanText(row.Contents().Not("*").Text()))
		value := extract.CleanText(row.Children().Text())
		if _, seen := rows[label]; label != "" && len(label) <= maxInfoLabelSize && !seen {
			rows[label] = value
		}
	})
	return rows
}

func altTitles(doc *goquery.Document) []string {
	titles := []string{}
	for _, t := range altTitleSepRe.Split(extract.CleanText(doc.Find(selAltTitles).First().Text()), -1) {
		if t != "" {
			titles = append(titles, t)
		}
	}
	return titles
}

// cover prefers the image in the info box (lazy attributes first), then
// og:image.
func cover(doc *goquery.Document) string {
	img := doc.Find(selCoverImg).First()
	for _, attr := range []string{"data-src", "data-lazy-src", "src"} {
		if v := strings.TrimSpace(img.AttrOr(attr, "")); v != "" && !strings.HasPrefix(v, "data:") {
			return v
		}
	}
	return strings.TrimSpace(doc.Find(selCoverMeta).First().AttrOr("content", ""))
}

func seriesType(raw string) domain.SeriesType {
	t := domain.SeriesType(strings.ToLower(raw))
	if t.Valid() {
		return t
	}
	return domain.TypeOther
}

// parseDate accepts the known absolute formats; relative dates such as
// "2 days ago" are left unknown rather than guessed.
func parseDate(s string) domain.Date {
	s = extract.CleanText(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return domain.NewDate(t.Year(), t.Month(), t.Day())
		}
	}
	return domain.Date{}
}
