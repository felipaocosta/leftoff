// Package mgeko is the native adapter for MangaGeko (mgeko.cc). The series
// page only shows recent chapters; the full list is on
// <series>/all-chapters/, with exact release dates in <time datetime>.
//
// It does NOT keep the site's synopsis: it is generic SEO text, not a
// description of the work.
package mgeko

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
	selTitle       = "h1.novel-title"
	selAltTitles   = "h2.alternative-title"
	selCoverImg    = "figure.cover img"
	selCoverMeta   = `meta[property="og:image"]`
	selStats       = ".header-stats span"
	selCategories  = ".categories a"
	selAllLink     = `a[href$="/all-chapters/"]`
	selChapterLink = "ul.chapter-list li a[href]"
	selChapterName = ".chapter-title"
	selChapterDate = "time[datetime]"
	allChapters    = "all-chapters/"
	placeholderImg = "default-placeholder"
)

var (
	seriesPathRe = regexp.MustCompile(`^/manga/[^/]+/?$`)
	// djangoDateRe reads "Sept. 14, 2026, 2:18 a.m." (Django's default format).
	djangoDateRe = regexp.MustCompile(`^([A-Za-z]{3})[a-z]*\.?\s+(\d{1,2}),\s+(\d{4})`)
	months       = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
)

func defaultHosts() []string { return []string{"mgeko.cc", "mgeko.com"} }

// Adapter implements extract.Adapter.
type Adapter struct{ hosts map[string]bool }

// New returns the adapter; hosts default to the known MangaGeko domains.
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
func (*Adapter) Name() string { return "mgeko" }

// Match accepts series pages of known hosts, or pages with the site's
// title block and "all chapters" link.
func (a *Adapter) Match(u *url.URL, doc *goquery.Document) bool {
	if doc == nil {
		return a.hosts[extract.Domain(u)] && seriesPathRe.MatchString(u.Path)
	}
	return doc.Find(selTitle).Length() > 0 && doc.Find(selAllLink).Length() > 0
}

// FetchSeries implements extract.Adapter.
func (*Adapter) FetchSeries(ctx context.Context, f fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error) {
	doc, err := load(ctx, f, seriesURL(u))
	if err != nil {
		return nil, err
	}
	title := extract.CleanText(doc.Find(selTitle).First().Text())
	if title == "" {
		return nil, fmt.Errorf("mgeko %s: title: %w", u, extract.ErrIncomplete)
	}
	return &domain.SeriesInfo{
		URL:       seriesURL(u),
		Title:     title,
		AltTitles: altTitles(doc),
		CoverURL:  extract.ResolveURL(u, cover(doc)),
		StatusRaw: status(doc),
		Type:      seriesType(doc),
	}, nil
}

// FetchChapters implements extract.Adapter, newest first as on the site.
func (*Adapter) FetchChapters(ctx context.Context, f fetch.Fetcher, u *url.URL) ([]domain.ChapterInfo, error) {
	doc, err := load(ctx, f, seriesURL(u)+allChapters)
	if err != nil {
		return nil, err
	}
	var chapters []domain.ChapterInfo
	doc.Find(selChapterLink).Each(func(_ int, a *goquery.Selection) {
		href := a.AttrOr("href", "")
		name := extract.CleanText(a.Find(selChapterName).First().Text())
		c := domain.ChapterInfo{URL: extract.ResolveURL(u, href), Label: name, Number: extract.ParseChapterNumber(href)}
		if c.Number != nil {
			// The site's label is the URL slug ("50-eng-li"); show a clean one.
			c.Label = "Chapter " + strconv.FormatFloat(*c.Number, 'f', -1, 64)
		}
		c.ReleasedAt = parseDate(a.Find(selChapterDate).First().AttrOr("datetime", ""))
		chapters = append(chapters, c)
	})
	return chapters, nil
}

// seriesURL is the canonical series URL with a trailing slash.
func seriesURL(u *url.URL) string { return strings.TrimSuffix(u.String(), "/") + "/" }

func altTitles(doc *goquery.Document) []string {
	out := []string{}
	for _, t := range strings.Split(extract.CleanText(doc.Find(selAltTitles).First().Text()), ",") {
		if t = strings.TrimSpace(t); t != "" && !strings.EqualFold(t, "Updating") {
			out = append(out, t)
		}
	}
	return out
}

// cover prefers the lazy-loaded image, skipping the placeholder.
func cover(doc *goquery.Document) string {
	img := doc.Find(selCoverImg).First()
	for _, attr := range []string{"data-src", "src"} {
		if v := strings.TrimSpace(img.AttrOr(attr, "")); v != "" && !strings.Contains(v, placeholderImg) {
			return v
		}
	}
	return strings.TrimSpace(doc.Find(selCoverMeta).First().AttrOr("content", ""))
}

// status reads "<strong>Ongoing</strong> <small>Status</small>".
func status(doc *goquery.Document) string {
	var raw string
	doc.Find(selStats).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if strings.EqualFold(extract.CleanText(s.Find("small").Text()), "status") {
			raw = extract.CleanText(s.Find("strong").Text())
			return false
		}
		return true
	})
	return raw
}

// seriesType reads the type among the categories ("Manhwa").
func seriesType(doc *goquery.Document) domain.SeriesType {
	t := domain.TypeOther
	doc.Find(selCategories).EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if c := domain.SeriesType(strings.ToLower(extract.CleanText(a.Text()))); c.Valid() && c != domain.TypeOther {
			t = c
			return false
		}
		return true
	})
	return t
}

// parseDate reads Django's "Sept. 14, 2026, 2:18 a.m."; anything else is
// left unknown.
func parseDate(s string) domain.Date {
	m := djangoDateRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return domain.Date{}
	}
	month, ok := months[strings.ToLower(m[1])]
	day, _ := strconv.Atoi(m[2])
	year, _ := strconv.Atoi(m[3])
	if !ok || day < 1 || day > 31 {
		return domain.Date{}
	}
	return domain.NewDate(year, time.Month(month), day)
}

func load(ctx context.Context, f fetch.Fetcher, rawURL string) (*goquery.Document, error) {
	resp, err := fetch.Get(ctx, f, rawURL)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("mgeko: parse %s: %w", rawURL, err)
	}
	return doc, nil
}
