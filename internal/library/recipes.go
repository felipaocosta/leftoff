package library

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/llm"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// RecipeBook is the set of recipes the extractor uses (recipe.Book).
type RecipeBook interface {
	Load(files map[string][]byte) []error
}

// RecipeStore keeps the user recipe files (implemented by *storage.Store).
type RecipeStore interface {
	Recipes() (map[string][]byte, error)
	SaveRecipe(ctx context.Context, domain string, data []byte) error
	DeleteRecipe(ctx context.Context, domain string) error
}

// RecipeGenerator writes a recipe for a page with a language model
// (implemented by *llm.Generator).
type RecipeGenerator interface {
	Generate(ctx context.Context, f fetch.Fetcher, p *extract.Page, site string) (*llm.Generated, error)
}

// ErrLLMDisabled means no language model is configured (OLLAMA_URL).
var ErrLLMDisabled = errors.New("no language model configured (set OLLAMA_URL and OLLAMA_MODEL)")

// RecipeInfo summarizes a stored recipe for listing.
type RecipeInfo struct {
	Domain      string    `json:"domain"`
	Aliases     []string  `json:"aliases"`
	GeneratedBy string    `json:"generated_by"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"` // set when the file is invalid
	// Failing are the series this recipe no longer reads (RF-55).
	Failing []FailingSeries `json:"failing"`
}

// RecipeTest is the outcome of trying a recipe on a page.
type RecipeTest struct {
	Result   *extract.Result `json:"result"`
	Problems []string        `json:"problems"`
}

// GeneratedRecipe is a recipe written by the language model, not saved:
// Recipe is its JSON text for the editor, tested on the page.
type GeneratedRecipe struct {
	Recipe   string          `json:"recipe"`
	Result   *extract.Result `json:"result"`
	Problems []string        `json:"problems"`
	Notes    []string        `json:"notes"` // what the recipe could do better
	Attempts int             `json:"attempts"`
}

// Recipes manages the user recipes (RF-53): list, read, save (validated),
// delete, test and generate them, reloading the extractor's book on every
// change.
type Recipes struct {
	store   RecipeStore
	book    RecipeBook
	fetcher fetch.Fetcher
	clock   domain.Clock
	gen     RecipeGenerator  // nil when no model is configured
	render  extract.Renderer // nil without a local browser
	series  SeriesLister     // nil: List does not report failing series
}

// minPlainChapters is how many chapter links a downloaded page needs before
// generation trusts it; below that, the page is rendered in the browser
// (the same rule as the extraction pipeline).
const minPlainChapters = 3

// NewRecipes builds the recipes service.
func NewRecipes(store RecipeStore, book RecipeBook, f fetch.Fetcher, clock domain.Clock) *Recipes {
	return &Recipes{store: store, book: book, fetcher: f, clock: clock}
}

// WithGenerator enables Generate (RF-54); call it while wiring.
func (r *Recipes) WithGenerator(g RecipeGenerator) *Recipes {
	r.gen = g
	return r
}

// WithRenderer lets Test and Generate read pages that only show their
// chapters with JavaScript (recipes with requires_browser).
func (r *Recipes) WithRenderer(rd extract.Renderer) *Recipes {
	r.render = rd
	return r
}

// CanGenerate reports whether a language model is configured.
func (r *Recipes) CanGenerate() bool { return r.gen != nil }

// List returns every stored recipe, invalid ones included with their error.
func (r *Recipes) List() ([]RecipeInfo, error) {
	files, err := r.store.Recipes()
	if err != nil {
		return nil, err
	}
	out := []RecipeInfo{}
	for name, data := range files {
		info := RecipeInfo{Domain: name, Aliases: []string{}, Failing: []FailingSeries{}}
		if rc, err := recipe.Parse(data); err != nil {
			info.Error = err.Error()
		} else {
			info.Aliases, info.GeneratedBy, info.UpdatedAt = nonNil(rc.Aliases), rc.GeneratedBy, rc.UpdatedAt
			info.Failing = r.failing(rc)
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out, nil
}

// Get returns the stored JSON of a recipe, or domain.ErrNotFound.
func (r *Recipes) Get(name string) ([]byte, error) {
	files, err := r.store.Recipes()
	if err != nil {
		return nil, err
	}
	data, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("recipe %s: %w", name, domain.ErrNotFound)
	}
	return data, nil
}

// Save validates and stores a recipe under its domain, then reloads.
func (r *Recipes) Save(ctx context.Context, name string, data []byte) error {
	rc, err := recipe.Parse(data)
	if err != nil {
		return fmt.Errorf("%w: %w", err, domain.ErrInvalidInput)
	}
	if rc.Domain != name {
		return fmt.Errorf("recipe domain %q does not match %q: %w", rc.Domain, name, domain.ErrInvalidInput)
	}
	if err := r.store.SaveRecipe(ctx, name, data); err != nil {
		return err
	}
	return r.reload()
}

// Delete removes a stored recipe, then reloads.
func (r *Recipes) Delete(ctx context.Context, name string) error {
	if err := r.store.DeleteRecipe(ctx, name); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("recipe %s: %w", name, domain.ErrNotFound)
		}
		return err
	}
	return r.reload()
}

// Test applies a recipe (stored or not) to the page at rawURL.
func (r *Recipes) Test(ctx context.Context, data []byte, rawURL string) (RecipeTest, error) {
	rc, err := recipe.Parse(data)
	if err != nil {
		return RecipeTest{}, fmt.Errorf("%w: %w", err, domain.ErrInvalidInput)
	}
	page, err := r.page(ctx, rawURL, rc.RequiresBrowser)
	if err != nil {
		return RecipeTest{}, err
	}
	res, problems, err := recipe.Test(ctx, rc, r.fetcher, page, r.clock)
	if err != nil {
		return RecipeTest{}, err
	}
	return RecipeTest{Result: res, Problems: nonNil(problems)}, nil
}

// Generate asks the language model for a recipe for site from the series
// page at rawURL (RF-54). The recipe is tested but NOT saved: the user
// reviews it and saves it (RULES §6.8).
func (r *Recipes) Generate(ctx context.Context, site, rawURL string) (GeneratedRecipe, error) {
	if r.gen == nil {
		return GeneratedRecipe{}, ErrLLMDisabled
	}
	u, err := extract.NormalizeURL(rawURL)
	if err != nil {
		return GeneratedRecipe{}, err
	}
	if host := strings.TrimPrefix(u.Hostname(), "www."); host != site {
		return GeneratedRecipe{}, fmt.Errorf("url host %q is not %q: %w", u.Hostname(), site, domain.ErrInvalidInput)
	}
	page, rendered, err := r.generationPage(ctx, u.String())
	if err != nil {
		return GeneratedRecipe{}, err
	}
	gen, err := r.gen.Generate(ctx, r.fetcher, page, site)
	if err != nil {
		return GeneratedRecipe{}, fmt.Errorf("generate recipe for %s: %w", site, err)
	}
	gen.Recipe.RequiresBrowser = rendered
	data, err := json.MarshalIndent(gen.Recipe, "", "  ")
	if err != nil {
		return GeneratedRecipe{}, fmt.Errorf("encode recipe: %w", err)
	}
	return GeneratedRecipe{Recipe: string(data) + "\n", Result: gen.Result, Problems: nonNil(gen.Problems), Notes: nonNil(gen.Notes), Attempts: gen.Attempts}, nil
}

// generationPage is the page the model reads: the downloaded one, or the
// one rendered in the browser when the download shows almost no chapters
// (rendered = true; the recipe then needs the browser too).
func (r *Recipes) generationPage(ctx context.Context, rawURL string) (*extract.Page, bool, error) {
	page, err := r.page(ctx, rawURL, false)
	if err != nil {
		return nil, false, err
	}
	if r.render == nil || extract.ChapterLinkCount(page.Doc, page.URL) >= minPlainChapters {
		return page, false, nil
	}
	if rendered, err := r.page(ctx, rawURL, true); err == nil {
		return rendered, true, nil
	}
	return page, false, nil // the browser failed: the downloaded page is still usable
}

// page downloads (or, with render and a browser, renders) and parses the
// page at rawURL.
func (r *Recipes) page(ctx context.Context, rawURL string, render bool) (*extract.Page, error) {
	u, err := extract.NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	var resp *fetch.Response
	if render && r.render != nil {
		resp, err = r.render.Render(ctx, u.String())
	} else {
		resp, err = fetch.Get(ctx, r.fetcher, u.String())
	}
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", u, err)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", u, err)
	}
	final := u
	if resp.URL != nil {
		final = resp.URL // after redirects
	}
	return &extract.Page{URL: final, Doc: doc}, nil
}

func (r *Recipes) reload() error {
	files, err := r.store.Recipes()
	if err != nil {
		return err
	}
	r.book.Load(files) // invalid files are skipped; List reports them
	return nil
}
