// Package mangadex is the native adapter for mangadex.org. MangaDex is a
// JavaScript app, so instead of the HTML it reads the official public API
// (api.mangadex.org), which the site provides for this kind of use.
//
// Chapters are read in the configured languages (default pt-br, then en);
// when a chapter exists in several, the preferred language comes first and
// the others become alt URLs. It does NOT read chapter pages or images.
package mangadex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

const (
	apiBase   = "https://api.mangadex.org"
	siteBase  = "https://mangadex.org"
	coverBase = "https://uploads.mangadex.org/covers"
	pageSize  = 500
	maxPages  = 20 // 10 000 chapters is far beyond any series
)

// titlePathRe matches /title/<uuid>[/slug].
var titlePathRe = regexp.MustCompile(`^/title/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:/|$)`)

// DefaultLanguages is the chapter language preference when none is given.
func DefaultLanguages() []string { return []string{"pt-br", "en"} }

// Adapter implements extract.Adapter.
type Adapter struct{ langs []string }

// New returns the adapter; langs are MangaDex language codes in order of
// preference (e.g. "pt-br", "en").
func New(langs ...string) *Adapter {
	if len(langs) == 0 {
		langs = DefaultLanguages()
	}
	return &Adapter{langs: langs}
}

// Name implements extract.Adapter.
func (*Adapter) Name() string { return "mangadex" }

// Match implements extract.Adapter: mangadex.org/title/<uuid> pages.
func (*Adapter) Match(u *url.URL, _ *goquery.Document) bool {
	return extract.Domain(u) == "mangadex.org" && titlePathRe.MatchString(u.Path)
}

func mangaID(u *url.URL) (string, error) {
	m := titlePathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", fmt.Errorf("mangadex: no title id in %s: %w", u, domain.ErrInvalidInput)
	}
	return m[1], nil
}

func getJSON(ctx context.Context, f fetch.Fetcher, rawURL string, v any) error {
	resp, err := fetch.Get(ctx, f, rawURL)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(resp.Body, v); err != nil {
		return fmt.Errorf("mangadex: decode %s: %w", rawURL, err)
	}
	return nil
}

// FetchSeries implements extract.Adapter.
func (a *Adapter) FetchSeries(ctx context.Context, f fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error) {
	id, err := mangaID(u)
	if err != nil {
		return nil, err
	}
	var body mangaResponse
	if err := getJSON(ctx, f, apiBase+"/manga/"+id+"?includes[]=cover_art", &body); err != nil {
		return nil, err
	}
	attr := body.Data.Attributes
	title := pick(attr.Title, append([]string{"en"}, a.langs...))
	if title == "" {
		return nil, fmt.Errorf("mangadex %s: title: %w", id, extract.ErrIncomplete)
	}
	return &domain.SeriesInfo{
		URL:       siteBase + "/title/" + id,
		Title:     title,
		AltTitles: altTitles(attr, title),
		CoverURL:  coverURL(id, body.Data.Relationships),
		StatusRaw: attr.Status,
		Synopsis:  pick(attr.Description, append(slices.Clone(a.langs), "en")),
		Type:      typeFor(attr.OriginalLanguage),
	}, nil
}

// FetchChapters implements extract.Adapter, newest first.
func (a *Adapter) FetchChapters(ctx context.Context, f fetch.Fetcher, u *url.URL) ([]domain.ChapterInfo, error) {
	id, err := mangaID(u)
	if err != nil {
		return nil, err
	}
	var all []chapterData
	for page := 0; page < maxPages; page++ {
		var body feedResponse
		if err := getJSON(ctx, f, a.feedURL(id, page*pageSize), &body); err != nil {
			return nil, err
		}
		all = append(all, body.Data...)
		if len(body.Data) < pageSize || len(all) >= body.Total {
			break
		}
	}
	a.sortByPreference(all)
	chapters := make([]domain.ChapterInfo, 0, len(all))
	for _, c := range all {
		chapters = append(chapters, toChapterInfo(c))
	}
	return chapters, nil
}

func (a *Adapter) feedURL(id string, offset int) string {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("order[chapter]", "desc")
	for _, l := range a.langs {
		q.Add("translatedLanguage[]", l)
	}
	for _, r := range []string{"safe", "suggestive", "erotica"} {
		q.Add("contentRating[]", r)
	}
	return apiBase + "/manga/" + id + "/feed?" + q.Encode()
}

// sortByPreference orders by chapter number (newest first) and, for the
// same number, by language preference, so normalization keeps the
// preferred translation and records the others as alt URLs.
func (a *Adapter) sortByPreference(all []chapterData) {
	rank := func(lang string) int {
		if i := slices.Index(a.langs, lang); i >= 0 {
			return i
		}
		return len(a.langs)
	}
	slices.SortStableFunc(all, func(x, y chapterData) int {
		nx, ny := parseNumber(x.Attributes.Chapter), parseNumber(y.Attributes.Chapter)
		switch {
		case nx != ny:
			if nx > ny {
				return -1
			}
			return 1
		default:
			return rank(x.Attributes.TranslatedLanguage) - rank(y.Attributes.TranslatedLanguage)
		}
	})
}

func toChapterInfo(c chapterData) domain.ChapterInfo {
	at := c.Attributes
	link := siteBase + "/chapter/" + c.ID
	if at.ExternalURL != "" {
		link = at.ExternalURL
	}
	label := "Chapter " + at.Chapter
	if at.Chapter == "" {
		label = "Oneshot"
	}
	if t := strings.TrimSpace(at.Title); t != "" {
		label += ": " + t
	}
	info := domain.ChapterInfo{Label: label, URL: link}
	if at.Chapter != "" {
		if n, err := strconv.ParseFloat(at.Chapter, 64); err == nil {
			info.Number = &n
		}
	}
	if t, err := time.Parse(time.RFC3339, at.PublishAt); err == nil {
		t = t.UTC()
		info.ReleasedAt = domain.NewDate(t.Year(), t.Month(), t.Day())
	}
	return info
}

// parseNumber sorts chapters without a number last.
func parseNumber(s string) float64 {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return n
}
