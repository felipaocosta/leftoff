package recipe

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
)

// Book holds the recipes in use, found by domain or alias. It is safe for
// concurrent use and can be reloaded when recipes change.
type Book struct {
	clock domain.Clock

	mu     sync.RWMutex
	byHost map[string]*Recipe
	all    []*Recipe
}

// NewBook returns an empty Book.
func NewBook(clock domain.Clock) *Book {
	return &Book{clock: clock, byHost: map[string]*Recipe{}}
}

// Load replaces the recipes with the given files (domain → JSON). Invalid
// files are skipped and reported; the valid ones are still loaded.
func (b *Book) Load(files map[string][]byte) []error {
	var errs []error
	byHost := map[string]*Recipe{}
	var all []*Recipe
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r, err := Parse(files[name])
		if err != nil {
			errs = append(errs, fmt.Errorf("recipe %s: %w", name, err))
			continue
		}
		all = append(all, r)
		for _, h := range append([]string{r.Domain}, r.Aliases...) {
			byHost[normalizeHost(h)] = r
		}
	}
	b.mu.Lock()
	b.byHost, b.all = byHost, all
	b.mu.Unlock()
	return errs
}

// Lookup implements extract.Recipes.
func (b *Book) Lookup(host string) (extract.RecipeRun, bool) {
	b.mu.RLock()
	r, ok := b.byHost[normalizeHost(host)]
	b.mu.RUnlock()
	if !ok {
		return extract.RecipeRun{}, false
	}
	return extract.RecipeRun{Extractor: NewExtractor(r, b.clock), Method: r.Method(), RequiresBrowser: r.RequiresBrowser}, true
}

// List returns the loaded recipes, sorted by domain.
func (b *Book) List() []*Recipe {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return append([]*Recipe(nil), b.all...)
}

func normalizeHost(h string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(h)), "www.")
}
