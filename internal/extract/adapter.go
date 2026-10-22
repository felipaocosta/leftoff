package extract

import (
	"context"
	"net/url"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Adapter extracts data from one kind of site (SPEC §5.2).
type Adapter interface {
	Name() string
	// Match decides whether this adapter can handle the page.
	// doc is nil when only the URL is known.
	Match(u *url.URL, doc *goquery.Document) bool
	FetchSeries(ctx context.Context, f fetch.Fetcher, u *url.URL) (*domain.SeriesInfo, error)
	FetchChapters(ctx context.Context, f fetch.Fetcher, u *url.URL) ([]domain.ChapterInfo, error)
}

// Page is a downloaded page handed to a Fallback extractor.
type Page struct {
	URL *url.URL // final URL after redirects
	Doc *goquery.Document
}

// Fallback extracts from a page no adapter matched (e.g. the heuristic).
// It returns a Result with its own Method; Confidence is recomputed.
type Fallback interface {
	Extract(ctx context.Context, f fetch.Fetcher, p *Page) (*Result, error)
}

// Renderer loads a page in a real browser and returns the DOM after its
// JavaScript ran (implemented by fetch/browser). It is used for pages whose
// content only appears with JavaScript.
type Renderer interface {
	Render(ctx context.Context, rawURL string) (*fetch.Response, error)
}

// Recipes finds the JSON recipe of a host (implemented by recipe.Book).
type Recipes interface {
	Lookup(host string) (RecipeRun, bool)
}

// RecipeRun is a recipe ready to apply to a downloaded page.
type RecipeRun struct {
	Extractor       Fallback // applies the recipe
	Method          string   // "recipe:site.org"
	RequiresBrowser bool     // render the page in the browser first
}

// Result is the output of an extraction.
type Result struct {
	Series     domain.SeriesInfo    `json:"series"`
	Chapters   []domain.ChapterInfo `json:"chapters"`
	Method     string               `json:"method"`     // "adapter:madara", "heuristic", ...
	Confidence float64              `json:"confidence"` // 0..1, see Confidence
}

// AdapterMethod is the Result.Method of a native adapter.
func AdapterMethod(a Adapter) string { return "adapter:" + a.Name() }
