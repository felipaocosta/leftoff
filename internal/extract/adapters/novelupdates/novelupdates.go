// Package novelupdates is the native adapter for novelupdates.com, an index
// of translated novels (SPEC §14). Each series page lists releases in a
// paginated table (?pg=N): date, group and a label such as "c12" or
// "v2c6 part4". The site is behind Cloudflare, so pages arrive through the
// fetch solvers (local browser or FlareSolverr).
//
// It does NOT use the chapter-list AJAX endpoint (Cloudflare blocks it) nor
// follow release links to the translators' sites.
package novelupdates

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
	selTitle      = ".seriestitlenu"
	selCover      = ".seriesimg img"
	selStatus     = "#editstatus"
	selAssociated = "#editassociated"
	selSynopsis   = "#editdescription"
	selReleaseRow = "#myTable tbody tr"
	selNextPage   = ".digg_pagination a.next_page"
	dateLayout    = "01/02/06"
	maxPages      = 50
)

var (
	seriesPathRe = regexp.MustCompile(`^/series/[^/]+/?$`)
	// plainChapterRe accepts labels that are just a chapter number ("c12",
	// "c12.5"); volume/part labels ("v2c6 part4") have no single number.
	plainChapterRe = regexp.MustCompile(`(?i)^c(?:h|hapter)?\.?\s*(\d+(?:\.\d+)?)$`)
)

// Adapter implements extract.Adapter.
type Adapter struct{}

// New returns the adapter.
func New() *Adapter { return &Adapter{} }

// Name implements extract.Adapter.
func (*Adapter) Name() string { return "novelupdates" }

// Match implements extract.Adapter: novelupdates.com/series/<slug>/.
func (*Adapter) Match(u *url.URL, _ *goquery.Document) bool {
	return extract.Domain(u) == "novelupdates.com" && seriesPathRe.MatchString(u.Path)
}

// FetchSeries implements extract.Adapter.
func (*Adapter) FetchSeries(ctx context.Context, f fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error) {
	doc, err := load(ctx, f, u.String())
	if err != nil {
		return nil, err
	}
	title := extract.CleanText(doc.Find(selTitle).First().Text())
	if title == "" {
		return nil, fmt.Errorf("novelupdates %s: title: %w", u, extract.ErrIncomplete)
	}
	return &domain.SeriesInfo{
		URL:       u.String(),
		Title:     title,
		AltTitles: associated(doc),
		CoverURL:  strings.TrimSpace(doc.Find(selCover).First().AttrOr("src", "")),
		StatusRaw: extract.CleanText(doc.Find(selStatus).First().Text()),
		Synopsis:  extract.CleanText(doc.Find(selSynopsis).First().Text()),
		Type:      domain.TypeNovel,
	}, nil
}

// FetchChapters implements extract.Adapter: every page of the release
// table, newest first.
func (*Adapter) FetchChapters(ctx context.Context, f fetch.Fetcher, u *url.URL) ([]domain.ChapterInfo, error) {
	var chapters []domain.ChapterInfo
	for page := 1; page <= maxPages; page++ {
		doc, err := load(ctx, f, pageURL(u, page))
		if err != nil {
			return nil, err
		}
		doc.Find(selReleaseRow).Each(func(_ int, tr *goquery.Selection) {
			if c, ok := release(tr, u); ok {
				chapters = append(chapters, c)
			}
		})
		if doc.Find(selNextPage).Length() == 0 {
			break
		}
	}
	return chapters, nil
}

// pageURL is the series URL for page 1 and <series>/?pg=N after it.
func pageURL(u *url.URL, page int) string {
	base := strings.TrimSuffix(u.String(), "/") + "/"
	if page == 1 {
		return base
	}
	return base + "?pg=" + strconv.Itoa(page)
}

// release reads one table row: date | group | label (with an optional link
// to the release).
func release(tr *goquery.Selection, series *url.URL) (domain.ChapterInfo, bool) {
	cells := tr.Find("td")
	if cells.Length() < 3 {
		return domain.ChapterInfo{}, false
	}
	last := cells.Eq(cells.Length() - 1)
	label := extract.CleanText(last.Find("[title]").First().AttrOr("title", ""))
	if label == "" {
		label = extract.CleanText(last.Text())
	}
	if label == "" {
		return domain.ChapterInfo{}, false
	}
	info := domain.ChapterInfo{Label: label}
	if m := plainChapterRe.FindStringSubmatch(label); m != nil {
		if n, err := strconv.ParseFloat(m[1], 64); err == nil {
			info.Number = &n
		}
	}
	// Without a release link the URL stays empty (the UI then opens the
	// series page): a shared URL would confuse the chapter merge.
	if href := last.Find("a[href]").First().AttrOr("href", ""); href != "" {
		info.URL = extract.ResolveURL(series, href)
	}
	if t, err := time.Parse(dateLayout, extract.CleanText(cells.Eq(0).Text())); err == nil {
		info.ReleasedAt = domain.NewDate(t.Year(), t.Month(), t.Day())
	}
	return info, true
}

// associated lists the alternative names, one per line.
func associated(doc *goquery.Document) []string {
	names := []string{}
	for _, n := range extract.TextChunks(doc.Find(selAssociated).First()) {
		if n != "" {
			names = append(names, n)
		}
	}
	return names
}

func load(ctx context.Context, f fetch.Fetcher, rawURL string) (*goquery.Document, error) {
	resp, err := fetch.Get(ctx, f, rawURL)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("novelupdates: parse %s: %w", rawURL, err)
	}
	return doc, nil
}
