package extract

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// fakeRenderer returns fixed HTML as the "rendered" page.
type fakeRenderer struct {
	html  string
	err   error
	calls int
}

func (r *fakeRenderer) Render(_ context.Context, rawURL string) (*fetch.Response, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	u, _ := url.Parse(rawURL)
	return &fetch.Response{URL: u, StatusCode: http.StatusOK, Body: []byte(r.html)}, nil
}

// chaptersFallback finds the title in <h1> and one chapter per <a>.
type chaptersFallback struct{}

func (chaptersFallback) Extract(_ context.Context, _ fetch.Fetcher, p *Page) (*Result, error) {
	res := &Result{Method: "heuristic", Series: domain.SeriesInfo{Title: p.Doc.Find("h1").Text(), CoverURL: "c.jpg", Status: domain.StatusOngoing}}
	for i := range p.Doc.Find("a").Length() {
		n := float64(i + 1)
		res.Chapters = append(res.Chapters, domain.ChapterInfo{Number: &n, Label: "Chapter", URL: "c"})
	}
	return res, nil
}

func TestExtract_RendersJavaScriptPages(t *testing.T) {
	tests := []struct {
		name       string
		static     string
		rendered   string
		renderErr  error
		wantMethod string
		wantCount  int
		wantCalls  int
		wantErr    error
	}{
		{"no chapters: rendered wins", `<h1>T</h1>`, `<h1>T</h1><a></a><a></a>`, nil, "heuristic+browser", 2, 1, nil},
		{"good static page: no rendering", `<h1>T</h1><a></a><a></a><a></a>`, ``, nil, "heuristic", 3, 0, nil},
		{"only a 'read' button: rendered wins", `<h1>T</h1><a></a>`, `<h1>T</h1><a></a><a></a><a></a><a></a>`, nil, "heuristic+browser", 4, 1, nil},
		{"rendering fails: static result kept", `<h1>T</h1>`, ``, errors.New("no browser"), "heuristic", 0, 1, nil},
		{"empty SPA shell: rendered rescues missing title", `<div id="app"></div>`, `<h1>Real</h1><a></a>`, nil, "heuristic+browser", 1, 1, nil},
		{"rendered is worse: static kept", `<h1>T</h1>`, `<h1>T</h1>`, nil, "heuristic", 0, 1, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRenderer{html: tt.rendered, err: tt.renderErr}
			f := &fetch.Static{Pages: map[string][]byte{page: []byte(tt.static)}}
			res, err := New(f, Options{Fallback: chaptersFallback{}, Renderer: r}).Extract(context.Background(), page)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v", err)
			}
			if r.calls != tt.wantCalls {
				t.Errorf("render calls = %d, want %d", r.calls, tt.wantCalls)
			}
			if err == nil && (res.Method != tt.wantMethod || len(res.Chapters) != tt.wantCount) {
				t.Errorf("method = %s, chapters = %d", res.Method, len(res.Chapters))
			}
		})
	}
}

func TestExtract_DoesNotRenderAdapterURLsOrBlockedPages(t *testing.T) {
	r := &fakeRenderer{html: `<h1>T</h1><a></a><a></a><a></a><a></a>`}
	ex := New(&fetch.Static{Pages: map[string][]byte{}}, Options{
		Adapters: []Adapter{&fakeAdapter{name: "api", host: "site.org", title: "T"}},
		Fallback: chaptersFallback{}, Renderer: r,
	})
	if _, err := ex.Extract(context.Background(), page); err != nil {
		t.Fatal(err)
	}
	if _, err := ex.Extract(context.Background(), "https://other.org/missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing page: %v", err)
	}
	if r.calls != 0 {
		t.Errorf("render calls = %d, want 0", r.calls)
	}
}
