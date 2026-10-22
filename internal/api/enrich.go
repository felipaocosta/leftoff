package api

import (
	"context"
	"net/http"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// Enricher links series to AniList or MangaUpdates (implemented by
// *library.Enricher). Nil disables the routes below.
type Enricher interface {
	Lookup(ctx context.Context, id string) (domain.Series, error)
	Unlink(ctx context.Context, id string) (domain.Series, error)
}

func (s *server) enrichRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/series/{id}/enrich", s.enrichSeries)
	mux.HandleFunc("DELETE /api/v1/series/{id}/external", s.unlinkSeries)
}

// enrichSeries looks the series up now (RF-57).
func (s *server) enrichSeries(w http.ResponseWriter, r *http.Request) {
	sr, err := s.opts.Enricher.Lookup(r.Context(), r.PathValue("id"))
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

// unlinkSeries removes a wrong AniList/MangaUpdates match.
func (s *server) unlinkSeries(w http.ResponseWriter, r *http.Request) {
	sr, err := s.opts.Enricher.Unlink(r.Context(), r.PathValue("id"))
	s.respondSeries(w, r, http.StatusOK, sr, err)
}
