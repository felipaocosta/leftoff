package madara

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Chapter list selectors and endpoints (RULES §6.6).
const (
	selChapterItem   = "li.wp-manga-chapter"
	selChapterLink   = "a[href]"
	selChapterDate   = ".chapter-release-date"
	selChapterNewTag = ".chapter-release-date a[title]" // "NEW" badge whose title is the date
	selChapterHolder = "#manga-chapters-holder"
	selMangaID       = "#manga-chapters-holder[data-id], .rating-post-id[value]"
	ajaxChaptersPath = "ajax/chapters/"           // newer theme: POST <series>/ajax/chapters/
	adminAjaxPath    = "/wp-admin/admin-ajax.php" // older theme: action=manga_get_chapters
	adminAjaxForm    = "action=manga_get_chapters&manga="
)

// FetchChapters implements extract.Adapter. Chapters come newest first, as
// on the page. When the page has no inline list, they are requested from
// the theme's AJAX endpoints.
func (a *Adapter) FetchChapters(ctx context.Context, f fetch.Fetcher, u *url.URL) ([]domain.ChapterInfo, error) {
	u = seriesURL(u)
	doc, err := load(ctx, f, u)
	if err != nil {
		return nil, err
	}
	if doc.Find(selChapterItem).Length() == 0 && doc.Find(selChapterHolder).Length() > 0 {
		if doc, err = ajaxChapters(ctx, f, u, doc); err != nil {
			return nil, err
		}
	}
	now := a.clock.Now()
	var chapters []domain.ChapterInfo
	doc.Find(selChapterItem).Each(func(_ int, li *goquery.Selection) {
		link := li.Find(selChapterLink).First()
		label := extract.CleanText(link.Text())
		href := link.AttrOr("href", "")
		if href == "" {
			return // a broken item (cut-off page): no chapter to open
		}
		n := extract.ParseChapterNumber(label)
		if n == nil {
			n = extract.ParseChapterNumber(href)
		}
		chapters = append(chapters, domain.ChapterInfo{Number: n, Label: label, URL: href, ReleasedAt: releaseDate(li, now)})
	})
	return chapters, nil
}

// ajaxChapters loads the list from POST <series>/ajax/chapters/ and, if
// that endpoint does not exist, from admin-ajax.php with the manga id.
func ajaxChapters(ctx context.Context, f fetch.Fetcher, u *url.URL, page *goquery.Document) (*goquery.Document, error) {
	base := *u
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	endpoint := base.ResolveReference(&url.URL{Path: ajaxChaptersPath}).String()
	doc, err := post(ctx, f, u, endpoint, "")
	if err == nil && doc.Find(selChapterItem).Length() > 0 {
		return doc, nil
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	id := mangaID(page)
	if id == "" {
		return nil, fmt.Errorf("madara %s: chapter list is loaded by AJAX and the manga id is missing: %w", u, extract.ErrIncomplete)
	}
	admin := u.ResolveReference(&url.URL{Path: adminAjaxPath}).String()
	return post(ctx, f, u, admin, adminAjaxForm+url.QueryEscape(id))
}

func post(ctx context.Context, f fetch.Fetcher, series *url.URL, endpoint, form string) (*goquery.Document, error) {
	h := http.Header{}
	h.Set("X-Requested-With", "XMLHttpRequest")
	h.Set("Referer", series.String())
	if form != "" {
		h.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	resp, err := f.Fetch(ctx, &fetch.Request{Method: http.MethodPost, URL: endpoint, Header: h, Body: []byte(form)})
	if err != nil {
		return nil, fmt.Errorf("madara chapters %s: %w", endpoint, err)
	}
	return parse(resp.Body, series)
}

// mangaID is the WordPress post id of the series, needed by admin-ajax.
func mangaID(doc *goquery.Document) string {
	el := doc.Find(selMangaID).First()
	if id := strings.TrimSpace(el.AttrOr("data-id", "")); id != "" {
		return id
	}
	return strings.TrimSpace(el.AttrOr("value", ""))
}

// releaseDate reads "13.10.2026", "October 15, 2026" or "3 hours ago"; for
// the newest chapters the date may only be the title of a "NEW" badge.
func releaseDate(li *goquery.Selection, now time.Time) domain.Date {
	if d, ok := extract.ParseReleaseDate(li.Find(selChapterDate).First().Text(), now); ok {
		return d
	}
	d, _ := extract.ParseReleaseDate(li.Find(selChapterNewTag).First().AttrOr("title", ""), now)
	return d
}
