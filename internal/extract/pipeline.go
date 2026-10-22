package extract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Options configures an Extractor. The adapter list is assembled explicitly
// in main (RULES §4: no init() registration).
type Options struct {
	Adapters []Adapter    // tried in order
	Fallback Fallback     // optional; used when no adapter matches
	Renderer Renderer     // optional; retries JavaScript-only pages in a browser
	Recipes  Recipes      // optional; JSON recipes by domain (SPEC §5.1 step 3)
	Logger   *slog.Logger // default discards
}

// RenderedSuffix marks results extracted from a browser-rendered page.
const RenderedSuffix = "+browser"

// Extractor runs the extraction pipeline (SPEC §5.1).
type Extractor struct {
	fetcher fetch.Fetcher
	opts    Options
}

// New builds an Extractor.
func New(f fetch.Fetcher, opts Options) *Extractor {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &Extractor{fetcher: f, opts: opts}
}

// Extract runs the pipeline for rawURL:
//
//  1. normalize the URL;
//  2. an adapter whose Match(url, nil) is true;
//  3. download the page and pick an adapter whose Match(url, doc) is true
//     (engine detection);
//  4. the fallback extractor, if any;
//  5. when the page result has no chapters or low confidence and a Renderer
//     is set, the fallback again on the browser-rendered page; the better
//     of the two results wins.
//
// Recipes and the LLM (SPEC §5.1 steps 3 and 6) plug in here in later phases.
func (e *Extractor) Extract(ctx context.Context, rawURL string) (*Result, error) {
	return e.extract(ctx, rawURL, true)
}

// ErrChapterPage means the URL is a chapter page whose series page could
// not be found; the user should paste the series link instead (RF-03).
var ErrChapterPage = errors.New("this is a chapter page and its series page was not found")

// extract runs the pipeline; chapter pages are resolved to their series
// only when followChapter is set (one level, no loops).
func (e *Extractor) extract(ctx context.Context, rawURL string, followChapter bool) (*Result, error) {
	u, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	if a := e.matchAdapter(u, nil); a != nil {
		return e.runAdapter(ctx, a, u)
	}

	page, err := e.download(ctx, u)
	if err != nil {
		return nil, err
	}
	if followChapter && isChapterPage(page.URL, page.Doc) {
		return e.fromChapterPage(ctx, page)
	}
	if res, ok := e.tryRecipe(ctx, page); ok {
		return res, nil
	}
	res, err := e.extractPage(ctx, page)
	if e.opts.Renderer == nil && looksUnrendered(res, err) {
		if err != nil { // keep the cause: still an ErrIncomplete
			return nil, fmt.Errorf("%w: %w", ErrNeedsBrowser, err)
		}
		return nil, fmt.Errorf("%s: %w", page.URL, ErrNeedsBrowser)
	}
	if !e.shouldRender(ctx, res, err) {
		return res, err
	}
	rendered, rerr := e.extractRendered(ctx, page.URL)
	if rerr != nil {
		e.opts.Logger.WarnContext(ctx, "browser rendering failed", "url", page.URL.String(), "err", rerr)
		return res, err
	}
	if err != nil || better(rendered, res) {
		return rendered, nil
	}
	return res, nil
}

// extractPage runs engine detection and the fallback on a downloaded page.
func (e *Extractor) extractPage(ctx context.Context, page *Page) (*Result, error) {
	if a := e.matchAdapter(page.URL, page.Doc); a != nil {
		res, err := e.runAdapter(ctx, a, page.URL)
		if err == nil || !e.canFallBack(err) {
			return res, err
		}
		e.opts.Logger.WarnContext(ctx, "adapter failed, trying fallback",
			"adapter", a.Name(), "url", page.URL.String(), "err", err)
	}
	return e.runFallback(ctx, page)
}

func (e *Extractor) matchAdapter(u *url.URL, doc *goquery.Document) Adapter {
	for _, a := range e.opts.Adapters {
		if a.Match(u, doc) {
			return a
		}
	}
	return nil
}

func (e *Extractor) runAdapter(ctx context.Context, a Adapter, u *url.URL) (*Result, error) {
	method := AdapterMethod(a)
	rec := newRedirectRecorder(e.fetcher, u)
	info, err := a.FetchSeries(ctx, rec, u)
	if err != nil {
		return nil, fmt.Errorf("%s: fetch series %s: %w", method, u, err)
	}
	chapters, err := a.FetchChapters(ctx, rec, u)
	if err != nil {
		return nil, fmt.Errorf("%s: fetch chapters %s: %w", method, u, err)
	}
	if moved := rec.movedURL(firstNonEmpty(info.URL, u.String())); moved != "" {
		info.URL = moved // the site moved to another domain (RF-08)
	}
	res := &Result{Series: *info, Chapters: chapters, Method: method}
	if err := normalizeResult(res, u); err != nil {
		return nil, err
	}
	return res, nil
}

func (e *Extractor) runFallback(ctx context.Context, page *Page) (*Result, error) {
	if e.opts.Fallback == nil {
		return nil, fmt.Errorf("extract %s: %w", page.URL, domain.ErrUnsupportedSite)
	}
	res, err := e.opts.Fallback.Extract(ctx, e.fetcher, page)
	if err != nil {
		return nil, fmt.Errorf("fallback extract %s: %w", page.URL, err)
	}
	if err := normalizeResult(res, page.URL); err != nil {
		return nil, err
	}
	capFewChapters(res) // adapters and recipes know the site; a guess with few chapters does not
	return res, nil
}

// canFallBack reports whether an adapter failure may be rescued by the
// fallback: parsing problems yes; site, network or cancellation errors no.
func (e *Extractor) canFallBack(err error) bool {
	if e.opts.Fallback == nil {
		return false
	}
	return !errors.Is(err, domain.ErrBlocked) &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded)
}

func (e *Extractor) download(ctx context.Context, u *url.URL) (*Page, error) {
	resp, err := fetch.Get(ctx, e.fetcher, u.String())
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html %s: %w", u, err)
	}
	final := u
	if resp.URL != nil {
		final = resp.URL
	}
	return &Page{URL: final, Doc: doc}, nil
}

// shouldRender reports whether a page result is poor enough to retry in a
// browser: (almost) no chapters, low confidence, or a parsing failure. Network,
// site and cancellation errors are not retried.
func (e *Extractor) shouldRender(ctx context.Context, res *Result, err error) bool {
	if e.opts.Renderer == nil || e.opts.Fallback == nil || ctx.Err() != nil {
		return false
	}
	if err != nil {
		return errors.Is(err, ErrIncomplete) || errors.Is(err, domain.ErrUnsupportedSite)
	}
	return len(res.Chapters) < fewChapters || res.Confidence < LowConfidence
}

// ErrNeedsBrowser means the page builds its content with JavaScript (an
// empty app shell: no title or no chapters) and no browser is available
// to render it, as on Android or a server without BROWSER_SOLVER.
var ErrNeedsBrowser = errors.New("the page needs a browser to show its content")

// looksUnrendered reports whether a guessed (non-adapter) result is an
// empty JavaScript shell: no title found, or no chapter at all.
func looksUnrendered(res *Result, err error) bool {
	if err != nil {
		return errors.Is(err, ErrIncomplete)
	}
	return len(res.Chapters) == 0 && !strings.HasPrefix(res.Method, "adapter:")
}

// fewChapters: a static page with fewer chapters usually shows only a
// "read first/latest" button while the real list loads with JavaScript.
const fewChapters = 3

// extractRendered runs the fallback on the browser-rendered page.
func (e *Extractor) extractRendered(ctx context.Context, u *url.URL) (*Result, error) {
	page, err := e.renderPage(ctx, u)
	if err != nil {
		return nil, err
	}
	res, err := e.runFallback(ctx, page)
	if err != nil {
		return nil, err
	}
	res.Method += RenderedSuffix
	return res, nil
}

// better prefers more chapters, then higher confidence.
func better(a, b *Result) bool {
	if len(a.Chapters) != len(b.Chapters) {
		return len(a.Chapters) > len(b.Chapters)
	}
	return a.Confidence > b.Confidence
}

// fromChapterPage finds the series of a chapter page (RF-03): it extracts
// each candidate page and keeps the first whose chapter list contains the
// chapter, so a site's home page listing other series is never mistaken for
// the series.
func (e *Extractor) fromChapterPage(ctx context.Context, page *Page) (*Result, error) {
	for _, candidate := range seriesCandidates(page.URL, page.Doc) {
		res, err := e.extract(ctx, candidate, false)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			e.opts.Logger.DebugContext(ctx, "series candidate failed", "url", candidate, "err", err)
			continue
		}
		if containsChapter(res, page.URL) {
			return res, nil
		}
	}
	return nil, fmt.Errorf("%s: %w", page.URL, ErrChapterPage)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// tryRecipe applies the host's recipe, if any (SPEC §5.1 step 3). A recipe
// that fails or finds no title or chapters is logged and skipped, so the
// adapters and the heuristic still get their turn.
func (e *Extractor) tryRecipe(ctx context.Context, page *Page) (*Result, bool) {
	if e.opts.Recipes == nil {
		return nil, false
	}
	run, ok := e.opts.Recipes.Lookup(page.URL.Hostname())
	if !ok {
		return nil, false
	}
	if run.RequiresBrowser && e.opts.Renderer != nil {
		if rendered, err := e.renderPage(ctx, page.URL); err == nil {
			page = rendered
		}
	}
	res, err := run.Extractor.Extract(ctx, e.fetcher, page)
	if err == nil {
		err = normalizeResult(res, page.URL)
	}
	if err == nil && len(res.Chapters) == 0 {
		err = fmt.Errorf("%s: no chapters: %w", run.Method, ErrIncomplete)
	}
	if err != nil {
		e.opts.Logger.WarnContext(ctx, "recipe failed, trying other extractors", "method", run.Method, "url", page.URL.String(), "err", err)
		return nil, false
	}
	res.Method = run.Method
	return res, true
}

// renderPage loads a page through the Renderer.
func (e *Extractor) renderPage(ctx context.Context, u *url.URL) (*Page, error) {
	resp, err := e.opts.Renderer.Render(ctx, u.String())
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse rendered html %s: %w", u, err)
	}
	final := u
	if resp.URL != nil {
		final = resp.URL
	}
	return &Page{URL: final, Doc: doc}, nil
}
