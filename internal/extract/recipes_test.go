package extract

import (
	"context"
	"testing"

	"github.com/felipaocosta/leftoff/internal/fetch"
)

// fakeRecipes serves one RecipeRun for site.org.
type fakeRecipes struct{ run RecipeRun }

func (f fakeRecipes) Lookup(host string) (RecipeRun, bool) {
	return f.run, host == "site.org"
}

func TestExtract_RecipeComesBeforeTheHeuristic(t *testing.T) {
	f := &fetch.Static{Pages: map[string][]byte{page: []byte(`<h1>T</h1><a href="/c/1">Chapter 1</a><a href="/c/2">Chapter 2</a><a href="/c/3">Chapter 3</a>`)}}
	recipes := fakeRecipes{RecipeRun{Extractor: linksFallback{}, Method: "recipe:site.org"}}
	res, err := New(f, Options{Fallback: chaptersFallback{}, Recipes: recipes}).Extract(context.Background(), page)
	if err != nil || res.Method != "recipe:site.org" || len(res.Chapters) != 3 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestExtract_FailingRecipeFallsBack(t *testing.T) {
	f := &fetch.Static{Pages: map[string][]byte{page: []byte(`<h1>T</h1><p>no chapter links</p><a></a><a></a><a></a>`)}}
	recipes := fakeRecipes{RecipeRun{Extractor: linksFallback{}, Method: "recipe:site.org"}} // finds no chapters
	res, err := New(f, Options{Fallback: chaptersFallback{}, Recipes: recipes}).Extract(context.Background(), page)
	if err != nil || res.Method != "heuristic" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestExtract_RecipeThatRequiresTheBrowser(t *testing.T) {
	f := &fetch.Static{Pages: map[string][]byte{page: []byte(`<div id="app"></div>`)}}
	r := &fakeRenderer{html: `<h1>T</h1><a href="/c/1">Chapter 1</a>`}
	recipes := fakeRecipes{RecipeRun{Extractor: linksFallback{}, Method: "recipe:site.org", RequiresBrowser: true}}
	res, err := New(f, Options{Recipes: recipes, Renderer: r}).Extract(context.Background(), page)
	if err != nil || res.Method != "recipe:site.org" || r.calls != 1 || len(res.Chapters) != 1 {
		t.Fatalf("res = %+v, renders = %d, err = %v", res, r.calls, err)
	}
}
