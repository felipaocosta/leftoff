package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const recipesDir = "recipes"

// ErrBadRecipeName means a recipe domain is not a plain host name.
var ErrBadRecipeName = errors.New("recipe name must be a domain")

var recipeNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)

// Recipes returns the user recipes in <DATA_DIR>/recipes as domain → JSON.
func (s *Store) Recipes() (map[string][]byte, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, recipesDir))
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	out := map[string][]byte{}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".json")
		if e.IsDir() || !ok || !recipeNameRe.MatchString(name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, recipesDir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read recipe %s: %w", name, err)
		}
		out[name] = data
	}
	return out, nil
}

// SaveRecipe writes <DATA_DIR>/recipes/<domain>.json atomically. The caller
// validates the content; storage only checks the name.
func (s *Store) SaveRecipe(ctx context.Context, domain string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save recipe: %w", err)
	}
	if !recipeNameRe.MatchString(domain) {
		return fmt.Errorf("save recipe %q: %w", domain, ErrBadRecipeName)
	}
	dir := filepath.Join(s.dir, recipesDir)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create recipes dir: %w", err)
	}
	return writeFileAtomic(filepath.Join(dir, domain+".json"), data, filePerm)
}

// DeleteRecipe removes a user recipe; a missing one is wrapped
// os.ErrNotExist.
func (s *Store) DeleteRecipe(ctx context.Context, domain string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete recipe: %w", err)
	}
	if !recipeNameRe.MatchString(domain) {
		return fmt.Errorf("delete recipe %q: %w", domain, ErrBadRecipeName)
	}
	if err := os.Remove(filepath.Join(s.dir, recipesDir, domain+".json")); err != nil {
		return fmt.Errorf("delete recipe %s: %w", domain, err)
	}
	return nil
}
