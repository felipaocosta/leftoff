package extract

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// fakeAdapter matches by host (URL-only) or by a CSS selector (engine
// detection) and returns canned data.
type fakeAdapter struct {
	name      string
	host      string // URL-only match when set
	selector  string // HTML match when set
	title     string
	seriesErr error
}

func (a *fakeAdapter) Name() string { return a.name }

func (a *fakeAdapter) Match(u *url.URL, doc *goquery.Document) bool {
	if doc == nil {
		return a.host != "" && u.Host == a.host
	}
	return a.selector != "" && doc.Find(a.selector).Length() > 0
}

func (a *fakeAdapter) FetchSeries(context.Context, fetch.Fetcher, *url.URL) (*domain.SeriesInfo, error) {
	if a.seriesErr != nil {
		return nil, a.seriesErr
	}
	return &domain.SeriesInfo{Title: a.title, StatusRaw: "Ongoing"}, nil
}

func (a *fakeAdapter) FetchChapters(context.Context, fetch.Fetcher, *url.URL) ([]domain.ChapterInfo, error) {
	return []domain.ChapterInfo{{Number: num(1), Label: "Chapter 1", URL: "chapter-1/"}}, nil
}

type fakeFallback struct{ called bool }

func (f *fakeFallback) Extract(_ context.Context, _ fetch.Fetcher, p *Page) (*Result, error) {
	f.called = true
	return &Result{
		Method: "heuristic", Series: domain.SeriesInfo{Title: p.Doc.Find("h1").Text()},
		Chapters: []domain.ChapterInfo{{Number: num(1), Label: "Chapter 1", URL: "chapter-1/"}},
	}, nil
}

const page = "https://site.org/manga/x/"

func staticSite(html string) *fetch.Static {
	return &fetch.Static{Pages: map[string][]byte{page: []byte(html)}}
}

func TestExtract_URLOnlyAdapterSkipsDownload(t *testing.T) {
	f := staticSite("")
	e := New(f, Options{Adapters: []Adapter{&fakeAdapter{name: "byurl", host: "site.org", title: "T"}}})
	res, err := e.Extract(context.Background(), page+"?utm_source=x")
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != "adapter:byurl" || res.Series.Status != domain.StatusOngoing || len(f.Requests) != 0 {
		t.Errorf("res = %+v, requests = %v", res, f.Requests)
	}
	if res.Chapters[0].URL != page+"chapter-1/" {
		t.Errorf("chapter URL not resolved: %q", res.Chapters[0].URL)
	}
}

func TestExtract_EngineDetectionByHTML(t *testing.T) {
	adapters := []Adapter{
		&fakeAdapter{name: "other", selector: ".nope", title: "wrong"},
		&fakeAdapter{name: "madara", selector: "div.wp-manga", title: "Right"},
	}
	e := New(staticSite(`<div class="wp-manga"></div>`), Options{Adapters: adapters})
	res, err := e.Extract(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != "adapter:madara" || res.Series.Title != "Right" || res.Confidence != 0.9 {
		t.Errorf("res = %+v", res)
	}
}

func TestExtract_FallbackWhenNoAdapterOrAdapterFails(t *testing.T) {
	tests := []struct {
		name     string
		adapters []Adapter
	}{
		{"no adapter matches", nil},
		{"adapter parse failure", []Adapter{&fakeAdapter{name: "m", selector: "h1", seriesErr: errors.New("layout changed")}}},
		{"adapter returns no title", []Adapter{&fakeAdapter{name: "m", selector: "h1"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &fakeFallback{}
			e := New(staticSite("<h1>Heur</h1>"), Options{Adapters: tt.adapters, Fallback: fb})
			res, err := e.Extract(context.Background(), page)
			if err != nil {
				t.Fatal(err)
			}
			if !fb.called || res.Method != "heuristic" || res.Series.Title != "Heur" {
				t.Errorf("res = %+v", res)
			}
		})
	}
}

func TestExtract_Errors(t *testing.T) {
	blocked := &fakeAdapter{name: "m", selector: "h1", seriesErr: domain.ErrBlocked}
	tests := []struct {
		name     string
		url      string
		adapters []Adapter
		fallback Fallback
		wantErr  error
	}{
		{"invalid url", "notaurl", nil, nil, domain.ErrInvalidInput},
		{"page not found", "https://site.org/missing", nil, nil, domain.ErrNotFound},
		{"no adapter and no fallback", page, nil, nil, domain.ErrUnsupportedSite},
		{"blocked is not rescued", page, []Adapter{blocked}, &fakeFallback{}, domain.ErrBlocked},
		{"fallback without title", "https://site.org/empty", nil, &fakeFallback{}, ErrIncomplete},
	}
	f := staticSite("<h1>x</h1>")
	f.Pages["https://site.org/empty"] = []byte("<p>nothing</p>")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(f, Options{Adapters: tt.adapters, Fallback: tt.fallback}).Extract(context.Background(), tt.url)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtract_AdapterErrorWithoutFallbackMentionsMethod(t *testing.T) {
	a := &fakeAdapter{name: "m", selector: "h1", seriesErr: errors.New("boom")}
	_, err := New(staticSite("<h1>x</h1>"), Options{Adapters: []Adapter{a}}).Extract(context.Background(), page)
	if err == nil || !strings.Contains(err.Error(), "adapter:m") {
		t.Errorf("err = %v, want it to name the adapter", err)
	}
}

// Without a browser, an empty JavaScript shell is reported as such, so
// the user is told what to do instead of "title not found" (Android).
func TestExtract_EmptyShellWithoutABrowserNeedsOne(t *testing.T) {
	shell := func(context.Context, fetch.Fetcher, *Page) (*Result, error) {
		return &Result{Method: "heuristic", Series: domain.SeriesInfo{Title: "Series"}}, nil
	}
	_, err := New(staticSite("<h1>Series</h1>"), Options{Fallback: fallbackFunc(shell)}).Extract(context.Background(), page)
	if !errors.Is(err, ErrNeedsBrowser) {
		t.Errorf("no chapters: err = %v, want ErrNeedsBrowser", err)
	}
	f := staticSite("<h1>x</h1>")
	f.Pages["https://site.org/empty"] = []byte("<div id=app></div>")
	_, err = New(f, Options{Fallback: &fakeFallback{}}).Extract(context.Background(), "https://site.org/empty")
	if !errors.Is(err, ErrNeedsBrowser) || !errors.Is(err, ErrIncomplete) {
		t.Errorf("no title: err = %v, want ErrNeedsBrowser wrapping ErrIncomplete", err)
	}
}

type fallbackFunc func(context.Context, fetch.Fetcher, *Page) (*Result, error)

func (f fallbackFunc) Extract(ctx context.Context, fe fetch.Fetcher, p *Page) (*Result, error) {
	return f(ctx, fe, p)
}
