// Package liliana is the native adapter for sites running the "Liliana"
// PHP manga theme, such as manhuaplus.org (SPEC §14). The series page
// already lists every chapter, so a single request serves both calls.
//
// It does NOT handle chapter pages (RF-03) nor paginated chapter lists.
package liliana

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
	selTitle        = "article header h1"
	selAltTitles    = "article header p"
	selInfoRow      = ".y6x11p"
	selInfoValue    = ".dt"
	selSynopsis     = "#syn-target"
	selChapterItem  = "ul#myUL li.chapter"
	selChapterLink  = "a[href]"
	selChapterTime  = "time[datetime]"
	altTitlesPrefix = "Other name:"
)

var (
	// seriesPathRe matches series pages (/manga/<slug>), not chapter pages.
	seriesPathRe = regexp.MustCompile(`^/manga/[^/]+/?$`)
	// altTitleSepRe splits alternative titles; commas are kept because
	// titles contain them ("Necromancer, the Ultimate Scourge!").
	altTitleSepRe = regexp.MustCompile(`\s*[•;]\s*`)
	coverMetas    = []string{`meta[property="og:image"]`, `meta[itemprop="image"]`}
)

// defaultHosts are known Liliana sites matched by URL alone.
func defaultHosts() []string { return []string{"manhuaplus.org"} }

// Adapter implements extract.Adapter.
type Adapter struct{ hosts map[string]bool }

// New returns the adapter. hosts lists sites matched by URL alone and
// defaults to the known Liliana sites (manhuaplus.org).
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
func (*Adapter) Name() string { return "liliana" }

// Match accepts series pages of known hosts by URL, or any page with the
// theme's chapter list and info rows.
func (a *Adapter) Match(u *url.URL, doc *goquery.Document) bool {
	if doc == nil {
		return a.hosts[extract.Domain(u)] && seriesPathRe.MatchString(u.Path)
	}
	return doc.Find(selChapterItem).Length() > 0 && doc.Find(selInfoRow).Length() > 0
}

// FetchSeries implements extract.Adapter.
func (*Adapter) FetchSeries(ctx context.Context, f fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error) {
	doc, err := load(ctx, f, u)
	if err != nil {
		return nil, err
	}
	title := extract.CleanText(doc.Find(selTitle).First().Text())
	if title == "" {
		return nil, fmt.Errorf("liliana %s: title: %w", u, extract.ErrIncomplete)
	}
	info := infoRows(doc)
	return &domain.SeriesInfo{
		URL:       u.String(),
		Title:     title,
		AltTitles: altTitles(doc),
		CoverURL:  cover(doc),
		StatusRaw: info["status"],
		Synopsis:  extract.CleanText(doc.Find(selSynopsis).First().Text()),
		Type:      seriesType(info["type"]),
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
		label := extract.CleanText(link.Text())
		href := link.AttrOr("href", "")
		if href == "" {
			return // a broken item (cut-off page): no chapter to open
		}
		number := extract.ParseChapterNumber(label)
		if number == nil {
			number = extract.ParseChapterNumber(href)
		}
		chapters = append(chapters, domain.ChapterInfo{
			Number:     number,
			Label:      label,
			URL:        href,
			ReleasedAt: unixDate(li.Find(selChapterTime).AttrOr("datetime", "")),
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
		return nil, fmt.Errorf("liliana: parse %s: %w", u, err)
	}
	return doc, nil
}

// infoRows maps lowercase row labels ("status", "type") to their values.
func infoRows(doc *goquery.Document) map[string]string {
	rows := map[string]string{}
	doc.Find(selInfoRow).Each(func(_ int, row *goquery.Selection) {
		value := row.Find(selInfoValue).First()
		label := strings.ToLower(extract.CleanText(row.Clone().Find(selInfoValue).Remove().End().Text()))
		if _, seen := rows[label]; label != "" && !seen {
			rows[label] = extract.CleanText(value.Text())
		}
	})
	return rows
}

func altTitles(doc *goquery.Document) []string {
	titles := []string{}
	doc.Find(selAltTitles).EachWithBreak(func(_ int, p *goquery.Selection) bool {
		text := extract.CleanText(p.Text())
		rest, ok := strings.CutPrefix(text, altTitlesPrefix)
		if !ok {
			return true
		}
		for _, t := range altTitleSepRe.Split(rest, -1) {
			if t = strings.TrimSpace(t); t != "" {
				titles = append(titles, t)
			}
		}
		return false
	})
	return titles
}

func cover(doc *goquery.Document) string {
	for _, sel := range coverMetas {
		if c := strings.TrimSpace(doc.Find(sel).First().AttrOr("content", "")); c != "" {
			return c
		}
	}
	return ""
}

func seriesType(raw string) domain.SeriesType {
	t := domain.SeriesType(strings.ToLower(raw))
	if t.Valid() {
		return t
	}
	return domain.TypeOther
}

// unixDate converts a Unix timestamp in seconds to a UTC date.
func unixDate(s string) domain.Date {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return domain.Date{}
	}
	t := time.Unix(n, 0).UTC()
	return domain.NewDate(t.Year(), t.Month(), t.Day())
}
