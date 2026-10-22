// Package madara is the native adapter for WordPress sites running the
// Madara theme (wp-manga), such as mangaread.org (SPEC §14, RF-52). The
// series page has the info box and, on most sites, the whole chapter list;
// sites that load the list by AJAX are read through the theme's chapter
// endpoints (chapters.go).
//
// A chapter link (/manga/<slug>/chapter-1/) is read as its series, the
// parent path (RF-03). It does NOT read chapter contents.
package madara

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Selectors and patterns (RULES §6.6).
const (
	selTitle       = ".post-title h1, .post-title h3"
	selTitleBadges = ".manga-title-badges"
	selCoverImg    = ".summary_image img"
	selCoverMeta   = `meta[property="og:image"]`
	selInfoItem    = ".post-content_item"
	selInfoLabel   = ".summary-heading"
	selInfoValue   = ".summary-content"
	selReadMore    = ".c-content-readmore, .content-readmore" // "Show more" button
	selThemeBox    = ".summary_image, .post-content_item"
	labelAlt       = "alternative"
	labelStatus    = "status"
	labelType      = "type"
)

var (
	// seriesPathRe matches series pages (/manga/<slug>/) and, with the
	// third group, chapter pages below them (/manga/<slug>/chapter-1/).
	seriesPathRe  = regexp.MustCompile(`^(/(?:manga|manhwa|manhua|webtoon|series)/[^/]+)(/[^/]+)?/?$`)
	altTitleSepRe = regexp.MustCompile(`\s*[;,•|/]\s*`)
	// synopsisSels are tried in order: the text box, then its wrappers.
	synopsisSels = []string{".summary__content", ".description-summary", ".manga-excerpt"}
)

func defaultHosts() []string { return []string{"mangaread.org"} }

// Adapter implements extract.Adapter.
type Adapter struct {
	hosts map[string]bool
	clock domain.Clock // turns "3 hours ago" into a date
}

// New returns the adapter; a nil clock is the system clock. hosts lists
// sites matched by URL alone and defaults to the known sites
// (mangaread.org).
func New(clock domain.Clock, hosts ...string) *Adapter {
	if clock == nil {
		clock = domain.SystemClock{}
	}
	if len(hosts) == 0 {
		hosts = defaultHosts()
	}
	a := &Adapter{hosts: map[string]bool{}, clock: clock}
	for _, h := range hosts {
		a.hosts[strings.ToLower(h)] = true
	}
	return a
}

// Name implements extract.Adapter.
func (*Adapter) Name() string { return "madara" }

// Match accepts series pages of known hosts by URL, or any page with the
// theme's info box and its chapter list (inline or the AJAX holder).
func (a *Adapter) Match(u *url.URL, doc *goquery.Document) bool {
	if doc == nil {
		return a.hosts[extract.Domain(u)] && seriesPathRe.MatchString(u.Path) // series or chapter
	}
	hasList := doc.Find(selChapterItem).Length() > 0 || doc.Find(selChapterHolder).Length() > 0
	return hasList && doc.Find(selThemeBox).Length() > 0
}

// FetchSeries implements extract.Adapter.
func (*Adapter) FetchSeries(ctx context.Context, f fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error) {
	u = seriesURL(u)
	doc, err := load(ctx, f, u)
	if err != nil {
		return nil, err
	}
	title := doc.Find(selTitle).First()
	title.Find(selTitleBadges).Remove() // "HOT", "NEW" badges inside the heading
	name := extract.CleanText(title.Text())
	if name == "" {
		return nil, fmt.Errorf("madara %s: title: %w", u, extract.ErrIncomplete)
	}
	info := infoItems(doc)
	return &domain.SeriesInfo{
		URL:       u.String(),
		Title:     name,
		AltTitles: splitTitles(info[labelAlt]),
		CoverURL:  cover(doc),
		StatusRaw: info[labelStatus],
		Synopsis:  synopsis(doc),
		Type:      seriesType(info[labelType]),
	}, nil
}

// seriesURL turns a chapter URL into its series URL (the parent path,
// with a trailing slash as the theme links it); other URLs are kept.
func seriesURL(u *url.URL) *url.URL {
	m := seriesPathRe.FindStringSubmatch(u.Path)
	if m == nil || m[2] == "" {
		return u
	}
	s := *u
	s.Path, s.RawQuery, s.Fragment = m[1]+"/", "", ""
	return &s
}

func load(ctx context.Context, f fetch.Fetcher, u *url.URL) (*goquery.Document, error) {
	resp, err := fetch.Get(ctx, f, u.String())
	if err != nil {
		return nil, err
	}
	return parse(resp.Body, u)
}

func parse(body []byte, u *url.URL) (*goquery.Document, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("madara: parse %s: %w", u, err)
	}
	return doc, nil
}

// infoItems maps lowercase labels of the info box ("status", "type",
// "alternative") to their values. Labels such as "Genre(s)" lose the "(s)".
func infoItems(doc *goquery.Document) map[string]string {
	items := map[string]string{}
	doc.Find(selInfoItem).Each(func(_ int, item *goquery.Selection) {
		label := strings.ToLower(extract.CleanText(item.Find(selInfoLabel).First().Text()))
		label = strings.TrimSuffix(label, "(s)")
		value := extract.CleanText(item.Find(selInfoValue).First().Text())
		if _, seen := items[label]; label != "" && value != "" && !seen {
			items[label] = value
		}
	})
	return items
}

func synopsis(doc *goquery.Document) string {
	for _, sel := range synopsisSels {
		box := doc.Find(sel).First()
		box.Find(selReadMore).Remove()
		if text := extract.CleanText(box.Text()); text != "" {
			return text
		}
	}
	return ""
}

func splitTitles(raw string) []string {
	titles := []string{}
	for _, t := range altTitleSepRe.Split(raw, -1) {
		if t = extract.CleanText(t); t != "" && !strings.EqualFold(t, "updating") {
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
