package recipe

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Extractor applies one recipe; it implements extract.Fallback.
type Extractor struct {
	r     *Recipe
	clock domain.Clock
}

// NewExtractor returns an extractor for a validated recipe.
func NewExtractor(r *Recipe, clock domain.Clock) *Extractor {
	if clock == nil {
		clock = domain.SystemClock{}
	}
	return &Extractor{r: r, clock: clock}
}

// Extract implements extract.Fallback: it reads the series from the page
// and the chapters from it or from chapters.list_url.
func (e *Extractor) Extract(ctx context.Context, f fetch.Fetcher, p *extract.Page) (*extract.Result, error) {
	s := e.r.Series
	info := domain.SeriesInfo{
		URL:       p.URL.String(),
		Title:     value(p.Doc.Selection, s.Title),
		CoverURL:  extract.ResolveURL(p.URL, value(p.Doc.Selection, s.Cover)),
		StatusRaw: value(p.Doc.Selection, s.Status),
		Synopsis:  value(p.Doc.Selection, s.Synopsis),
		AltTitles: list(p.Doc.Selection, s.AltTitles),
		Type:      domain.TypeOther,
	}
	listDoc, listURL, err := e.chapterPage(ctx, f, p)
	if err != nil {
		return nil, err
	}
	return &extract.Result{Series: info, Chapters: e.chapters(listDoc, listURL), Method: e.r.Method()}, nil
}

// chapterPage returns the page holding the chapter list.
func (e *Extractor) chapterPage(ctx context.Context, f fetch.Fetcher, p *extract.Page) (*goquery.Document, *url.URL, error) {
	if e.r.Chapters.ListURL == nil || strings.TrimSpace(*e.r.Chapters.ListURL) == "" {
		return p.Doc, p.URL, nil
	}
	raw := extract.ResolveURL(withSlash(p.URL), *e.r.Chapters.ListURL)
	u, err := url.Parse(raw)
	if raw == "" || err != nil {
		return nil, nil, fmt.Errorf("%s: bad chapters.list_url %q", e.r.Method(), *e.r.Chapters.ListURL)
	}
	resp, err := fetch.Get(ctx, f, raw)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: chapter list: %w", e.r.Method(), err)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: parse chapter list: %w", e.r.Method(), err)
	}
	return doc, u, nil
}

func (e *Extractor) chapters(doc *goquery.Document, base *url.URL) []domain.ChapterInfo {
	c := e.r.Chapters
	var numRe *regexp.Regexp
	if c.NumberRegex != "" {
		numRe = regexp.MustCompile(c.NumberRegex) // validated by Parse
	}
	now := e.clock.Now()
	var out []domain.ChapterInfo
	doc.Find(c.Item.CSS).Each(func(_ int, item *goquery.Selection) {
		link := item
		if c.Link.CSS != "" {
			link = item.Find(c.Link.CSS).First()
		}
		attr := c.Link.Attr
		if attr == "" {
			attr = "href"
		}
		href := extract.ResolveURL(base, link.AttrOr(attr, ""))
		label := extract.CleanText(item.Text())
		if c.Label.CSS != "" {
			label = value(item, c.Label)
		}
		info := domain.ChapterInfo{Label: label, URL: href, Number: number(numRe, label, href)}
		if c.Date.CSS != "" {
			info.ReleasedAt, _ = extract.ParseReleaseDate(value(item, c.Date), now)
		}
		if info.Label != "" || info.Number != nil {
			out = append(out, info)
		}
	})
	return out
}

// number reads the chapter number with the recipe's regex (first capture
// group) and falls back to the common parser on the label and the URL.
func number(re *regexp.Regexp, label, href string) *float64 {
	if re != nil {
		for _, s := range []string{label, href} {
			if m := re.FindStringSubmatch(s); m != nil {
				if n, err := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64); err == nil {
					return &n
				}
			}
		}
	}
	if n := extract.ParseChapterNumber(label); n != nil {
		return n
	}
	return extract.ParseChapterNumber(href)
}

// value is the first match of f under sel: its attribute or its text.
func value(sel *goquery.Selection, f Field) string {
	if f.CSS == "" {
		return ""
	}
	el := sel.Find(f.CSS).First()
	if f.Attr != "" {
		v := strings.TrimSpace(el.AttrOr(f.Attr, ""))
		if f.Attr == "src" && (v == "" || strings.HasPrefix(v, "data:")) {
			v = strings.TrimSpace(el.AttrOr("data-src", v)) // lazy-loaded images
		}
		return v
	}
	return extract.CleanText(el.Text())
}

// list is the split value of f, or the text of every match when no
// separator is given.
func list(sel *goquery.Selection, f Field) []string {
	out := []string{}
	if f.CSS == "" {
		return out
	}
	var parts []string
	if f.Split != "" {
		parts = strings.Split(value(sel, f), f.Split)
	} else {
		sel.Find(f.CSS).Each(func(_ int, s *goquery.Selection) { parts = append(parts, s.Text()) })
	}
	for _, p := range parts {
		if p = extract.CleanText(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func withSlash(u *url.URL) *url.URL {
	c := *u
	if !strings.HasSuffix(c.Path, "/") {
		c.Path += "/"
	}
	return &c
}

// Test applies the recipe to a downloaded page and reports what is
// missing, so a recipe is only saved when it works (SPEC §5.5).
func Test(ctx context.Context, r *Recipe, f fetch.Fetcher, p *extract.Page, clock domain.Clock) (*extract.Result, []string, error) {
	res, err := NewExtractor(r, clock).Extract(ctx, f, p)
	if err != nil {
		return nil, nil, err
	}
	var problems []string
	if res.Series.Title == "" {
		problems = append(problems, "series.title found nothing")
	}
	if len(res.Chapters) == 0 {
		problems = append(problems, "chapters.item found no chapters")
	} else if countNumbers(res.Chapters) == 0 {
		problems = append(problems, "no chapter has a number (check chapters.label or number_regex)")
	}
	return res, problems, nil
}

func countNumbers(chs []domain.ChapterInfo) int {
	n := 0
	for _, c := range chs {
		if c.Number != nil {
			n++
		}
	}
	return n
}
