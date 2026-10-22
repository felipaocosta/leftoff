package library

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/llm"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/storage"
)

func newRecipesEnv(t *testing.T) (*Recipes, *recipe.Book, *storage.Store) {
	t.Helper()
	store, err := storage.Open(context.Background(), t.TempDir(), domain.FixedClock{T: t0}, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	html, err := testfixture.ReadFile(t, "../../testdata/fixtures/ravenscans.org/the-extras-academy-survival-guide.html")
	if err != nil {
		t.Fatal(err)
	}
	f := &fetch.Static{Pages: map[string][]byte{"https://ravenscans.org/manga/the-extras-academy-survival-guide/": html}}
	book := recipe.NewBook(domain.FixedClock{T: t0})
	return NewRecipes(store, book, f, domain.FixedClock{T: t0}), book, store
}

func TestRecipes_SaveListGetDelete(t *testing.T) {
	r, book, store := newRecipesEnv(t)
	ctx := context.Background()
	data, err := testfixture.ReadFile(t, "../../testdata/recipes/ravenscans.org.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Save(ctx, "ravenscans.org", data); err != nil {
		t.Fatal(err)
	}
	if _, ok := book.Lookup("ravenscans.com"); !ok {
		t.Error("saving must reload the book (alias lookup)")
	}
	if got, err := r.Get("ravenscans.org"); err != nil || string(got) != string(data) {
		t.Errorf("Get = %q, %v", got, err)
	}

	// An invalid file written by hand is listed with its error.
	if err := store.SaveRecipe(ctx, "broken.org", []byte(`{"schema":1}`)); err != nil {
		t.Fatal(err)
	}
	list, err := r.List()
	if err != nil || len(list) != 2 || list[0].Domain != "broken.org" || list[0].Error == "" ||
		list[1].GeneratedBy != "manual" || len(list[1].Aliases) != 1 {
		t.Errorf("list = %+v, %v", list, err)
	}

	if err := r.Delete(ctx, "ravenscans.org"); err != nil {
		t.Fatal(err)
	}
	if _, ok := book.Lookup("ravenscans.org"); ok {
		t.Error("deleting must reload the book")
	}
	if err := r.Delete(ctx, "ravenscans.org"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("delete again: %v", err)
	}
	if _, err := r.Get("nope.org"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("get missing: %v", err)
	}
}

func TestRecipes_SaveRejectsInvalidOrMismatched(t *testing.T) {
	r, _, _ := newRecipesEnv(t)
	ctx := context.Background()
	if err := r.Save(ctx, "x.org", []byte(`{"schema":1}`)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("invalid: %v", err)
	}
	data, _ := testfixture.ReadFile(t, "../../testdata/recipes/ravenscans.org.json")
	if err := r.Save(ctx, "other.org", data); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("domain mismatch: %v", err)
	}
}

func TestRecipes_TestAgainstARealPage(t *testing.T) {
	r, _, _ := newRecipesEnv(t)
	data, _ := testfixture.ReadFile(t, "../../testdata/recipes/ravenscans.org.json")
	got, err := r.Test(context.Background(), data, "https://ravenscans.org/manga/the-extras-academy-survival-guide/")
	if err != nil || len(got.Problems) != 0 || len(got.Result.Chapters) != 130 {
		t.Fatalf("test = %+v, %v", got, err)
	}
	if _, err := r.Test(context.Background(), data, "https://ravenscans.org/manga/missing/"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing page: %v", err)
	}
	if _, err := r.Test(context.Background(), []byte(`{}`), "https://a.org"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("invalid recipe: %v", err)
	}
}

// fakeGenerator returns the ravenscans test recipe, or err.
type fakeGenerator struct {
	err  error
	site string
}

func (g *fakeGenerator) Generate(ctx context.Context, f fetch.Fetcher, p *extract.Page, site string) (*llm.Generated, error) {
	g.site = site
	if g.err != nil {
		return nil, g.err
	}
	data, err := os.ReadFile("../../testdata/recipes/ravenscans.org.json") // the tests using it Require the folder
	if err != nil {
		return nil, err
	}
	rc, err := recipe.Parse(data)
	if err != nil {
		return nil, err
	}
	res, problems, err := recipe.Test(ctx, rc, f, p, domain.FixedClock{T: t0})
	return &llm.Generated{Recipe: rc, Result: res, Problems: problems, Attempts: 1}, err
}

func TestRecipes_GenerateTestsButDoesNotSave(t *testing.T) {
	testfixture.Require(t) // the fake model answers with a saved recipe
	r, _, store := newRecipesEnv(t)
	ctx := context.Background()
	const page = "https://www.ravenscans.org/manga/the-extras-academy-survival-guide/"
	if _, err := r.Generate(ctx, "ravenscans.org", page); !errors.Is(err, ErrLLMDisabled) || r.CanGenerate() {
		t.Fatalf("without a model: %v", err)
	}
	gen := &fakeGenerator{}
	r.WithGenerator(gen)
	if _, err := r.Generate(ctx, "other.org", page); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("host mismatch: %v", err)
	}

	// The fixture is served without www; the generator gets the bare site.
	got, err := r.Generate(ctx, "ravenscans.org", strings.Replace(page, "www.", "", 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recipe.Parse([]byte(got.Recipe)); err != nil || gen.site != "ravenscans.org" ||
		len(got.Result.Chapters) != 130 || got.Problems == nil || got.Notes == nil || got.Attempts != 1 {
		t.Errorf("generated = %+v, parse err %v", got, err)
	}
	if files, _ := store.Recipes(); len(files) != 0 {
		t.Errorf("Generate must not save: %v", files)
	}

	gen.err = llm.ErrNoRecipe
	if _, err := r.Generate(ctx, "ravenscans.org", strings.Replace(page, "www.", "", 1)); !errors.Is(err, llm.ErrNoRecipe) {
		t.Errorf("generator error: %v", err)
	}
}

// fakeRenderer serves pages "rendered in a browser", or fails.
type fakeRenderer struct {
	pages map[string][]byte
	calls int
}

func (r *fakeRenderer) Render(_ context.Context, rawURL string) (*fetch.Response, error) {
	r.calls++
	body, ok := r.pages[rawURL]
	if !ok {
		return nil, errors.New("browser failed")
	}
	return &fetch.Response{Body: body}, nil // no URL: the requested one is kept
}

func TestRecipes_JavaScriptPagesUseTheBrowser(t *testing.T) {
	const page = "https://ravenscans.org/manga/js-only/"
	html, err := testfixture.ReadFile(t, "../../testdata/fixtures/ravenscans.org/the-extras-academy-survival-guide.html")
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(context.Background(), t.TempDir(), domain.FixedClock{T: t0}, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Downloaded without a browser, the page has no chapter list yet.
	f := &fetch.Static{Pages: map[string][]byte{page: []byte(`<html><h1 class="entry-title">X</h1></html>`)}}
	rd := &fakeRenderer{pages: map[string][]byte{page: html}}
	r := NewRecipes(store, recipe.NewBook(domain.FixedClock{T: t0}), f, domain.FixedClock{T: t0}).
		WithGenerator(&fakeGenerator{}).WithRenderer(rd)
	ctx := context.Background()

	got, err := r.Generate(ctx, "ravenscans.org", page)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := recipe.Parse([]byte(got.Recipe))
	if err != nil || !rc.RequiresBrowser || len(got.Result.Chapters) != 130 || rd.calls != 1 {
		t.Errorf("generate: requires_browser %v, %d chapters, %d renders, %v", rc != nil && rc.RequiresBrowser, len(got.Result.Chapters), rd.calls, err)
	}

	// Testing a requires_browser recipe renders the page too.
	test, err := r.Test(ctx, []byte(got.Recipe), page)
	if err != nil || len(test.Result.Chapters) != 130 || rd.calls != 2 {
		t.Errorf("test: %d chapters, %d renders, %v", len(test.Result.Chapters), rd.calls, err)
	}

	// When the browser fails, the downloaded page is used as it is.
	rd.pages = nil
	got, err = r.Generate(ctx, "ravenscans.org", page)
	if err != nil {
		t.Fatal(err)
	}
	if rc, _ := recipe.Parse([]byte(got.Recipe)); rc.RequiresBrowser || len(got.Result.Chapters) != 0 {
		t.Errorf("browser failure: %+v", got)
	}
}
