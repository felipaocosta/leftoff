package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// maxRecipeBytes limits recipe uploads; recipes are small JSON files.
const maxRecipeBytes = 256 << 10

// generateWriteTimeout replaces the server's write timeout for recipe
// generation: a local model on a CPU may take minutes.
const generateWriteTimeout = 15 * time.Minute

func (s *server) recipeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/recipes", s.listRecipes)
	mux.HandleFunc("GET /api/v1/recipes/{domain}", s.getRecipe)
	mux.HandleFunc("PUT /api/v1/recipes/{domain}", s.putRecipe)
	mux.HandleFunc("DELETE /api/v1/recipes/{domain}", s.deleteRecipe)
	mux.HandleFunc("POST /api/v1/recipes/{domain}/test", s.testRecipe)
	mux.HandleFunc("POST /api/v1/recipes/{domain}/generate", s.generateRecipe)
}

func (s *server) listRecipes(w http.ResponseWriter, r *http.Request) {
	list, err := s.opts.Recipes.List()
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipes": list})
}

func (s *server) getRecipe(w http.ResponseWriter, r *http.Request) {
	data, err := s.opts.Recipes.Get(r.PathValue("domain"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data) // the client may be gone; nothing left to do
}

// putRecipe stores the raw JSON body as the recipe of {domain}.
func (s *server) putRecipe(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRecipeBytes))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.opts.Recipes.Save(r.Context(), r.PathValue("domain"), data); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) deleteRecipe(w http.ResponseWriter, r *http.Request) {
	if err := s.opts.Recipes.Delete(r.Context(), r.PathValue("domain")); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// testRecipe tries {url, recipe} (the recipe need not be saved).
func (s *server) testRecipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string          `json:"url"`
		Recipe json.RawMessage `json:"recipe"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	if len(req.Recipe) == 0 {
		s.writeError(w, r, fmt.Errorf("recipe is required: %w", domain.ErrInvalidInput))
		return
	}
	res, err := s.opts.Recipes.Test(r.Context(), req.Recipe, req.URL)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// generateRecipe asks the language model for a recipe from {url}; the
// recipe is returned for review, not saved (RF-54).
func (s *server) generateRecipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	// Not every ResponseWriter supports deadlines (e.g. in tests).
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(generateWriteTimeout))
	gen, err := s.opts.Recipes.Generate(r.Context(), r.PathValue("domain"), req.URL)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, gen)
}
