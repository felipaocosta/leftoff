package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/llm"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/library"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// scriptedModel always gives the same answer.
type scriptedModel struct{ reply string }

func (m scriptedModel) Model() string { return "test-model" }

func (m scriptedModel) Complete(context.Context, []llm.Message, json.RawMessage) (string, error) {
	return m.reply, nil
}

// manhuaplusDraft is a model answer that works on the manhuaplus fixture.
const manhuaplusDraft = `{"series":{"title":{"css":"article header h1","attr":"","split":""},
"cover":{"css":"meta[property='og:image']","attr":"content","split":""},
"status":{"css":"","attr":"","split":""},"synopsis":{"css":"","attr":"","split":""},
"alt_titles":{"css":"","attr":"","split":""}},
"chapters":{"list_url":"","item":{"css":"ul#myUL li.chapter","attr":"","split":""},
"link":{"css":"a","attr":"href","split":""},"label":{"css":"a","attr":"","split":""},
"number_regex":"","date":{"css":"","attr":"","split":""}}}`

// newRecipesAPI wires recipes with a language model that answers reply
// (none when reply is empty).
func newRecipesAPI(t *testing.T, reply string) (*storage.Store, func(method, path, body string) response) {
	t.Helper()
	html, err := testfixture.ReadFile(t, "../../testdata/fixtures/manhuaplus.org/forging-an-industrial-empire-in-a-magical-world.html")
	if err != nil {
		t.Fatal(err)
	}
	f := &fetch.Static{Pages: map[string][]byte{seriesURL: html}}
	clock := domain.FixedClock{T: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
	store, err := storage.Open(context.Background(), t.TempDir(), clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	recipes := library.NewRecipes(store, recipe.NewBook(clock), f, clock)
	if reply != "" {
		recipes.WithGenerator(llm.NewGenerator(scriptedModel{reply}, clock, 0))
	}
	svc := library.New(store, extract.New(f, extract.Options{}), clock, library.Options{Fetcher: f})
	h := New(svc, store, Options{Version: "test", Recipes: recipes})
	return store, func(method, path, body string) response { return do(t, h, method, path, body) }
}

func TestGenerateRecipe_WithoutModel(t *testing.T) {
	_, call := newRecipesAPI(t, "")
	if r := call("GET", "/api/v1/health", ""); r.json(t)["llm"] != false {
		t.Errorf("health: %s", r.body)
	}
	r := call("POST", "/api/v1/recipes/manhuaplus.org/generate", `{"url":"`+seriesURL+`"}`)
	if r.status != 503 || r.errorCode(t) != "llm_disabled" {
		t.Errorf("got %d %s", r.status, r.body)
	}
}

func TestGenerateRecipe_ReturnsATestedRecipeWithoutSaving(t *testing.T) {
	store, call := newRecipesAPI(t, manhuaplusDraft)
	if r := call("GET", "/api/v1/health", ""); r.json(t)["llm"] != true {
		t.Errorf("health: %s", r.body)
	}
	r := call("POST", "/api/v1/recipes/manhuaplus.org/generate", `{"url":"`+seriesURL+`"}`)
	if r.status != 200 {
		t.Fatalf("got %d %s", r.status, r.body)
	}
	body := r.json(t)
	text, _ := body["recipe"].(string)
	rc, err := recipe.Parse([]byte(text))
	if err != nil || rc.GeneratedBy != "llm:test-model" || rc.Domain != "manhuaplus.org" {
		t.Errorf("recipe %q: %v", text, err)
	}
	res, _ := body["result"].(map[string]any)
	if chs, _ := res["chapters"].([]any); len(chs) != 81 || len(body["problems"].([]any)) != 0 || body["attempts"] != 1.0 {
		t.Errorf("body = %s", r.body)
	}
	if files, _ := store.Recipes(); len(files) != 0 {
		t.Error("generate must not save the recipe")
	}

	if r := call("POST", "/api/v1/recipes/other.org/generate", `{"url":"`+seriesURL+`"}`); r.errorCode(t) != "invalid_input" {
		t.Errorf("host mismatch: %d %s", r.status, r.body)
	}
}

func TestGenerateRecipe_ModelNeverWorks(t *testing.T) {
	_, call := newRecipesAPI(t, "I cannot help with that")
	r := call("POST", "/api/v1/recipes/manhuaplus.org/generate", `{"url":"`+seriesURL+`"}`)
	if r.status != 422 || r.errorCode(t) != "no_recipe" || !strings.Contains(string(r.body), "valid recipe") {
		t.Errorf("got %d %s", r.status, r.body)
	}
}
