package library

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/enrich"
)

// How often a series is looked up again (RF-57): matches age slowly,
// misses are retried less often to spare the public APIs.
const (
	enrichFoundEvery = 7 * 24 * time.Hour
	enrichMissEvery  = 30 * 24 * time.Hour
	enrichTimeout    = 20 * time.Second
)

// AfterSaver runs after a series is added or refreshed (set with
// UseEnricher). It must not fail the add or refresh.
type AfterSaver interface {
	AfterSave(ctx context.Context, id string)
}

// UseEnricher makes Add and Refresh call e afterwards; call it while
// wiring, before serving requests.
func (s *Service) UseEnricher(e AfterSaver) { s.enricher = e }

func (s *Service) afterSave(ctx context.Context, id string) {
	if s.enricher != nil {
		s.enricher.AfterSave(ctx, id)
	}
}

// Enricher links series to AniList or MangaUpdates and keeps the official
// status, genres, year and synopsis in series.external (RF-57). Automatic
// lookups only run when enrichment is enabled; the user can always ask.
type Enricher struct {
	lib      *Service
	settings *Settings
	finders  []enrich.Finder
	log      *slog.Logger
}

// NewEnricher builds the enricher with finders tried in order.
func NewEnricher(lib *Service, settings *Settings, finders []enrich.Finder, logger *slog.Logger) *Enricher {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Enricher{lib: lib, settings: settings, finders: finders, log: logger}
}

// AfterSave implements AfterSaver: it looks the series up when enrichment
// is on and the last lookup is old. Failures are only logged.
func (e *Enricher) AfterSave(ctx context.Context, id string) {
	if !e.settings.Enrichment().Enabled {
		return
	}
	sr, err := e.lib.Get(id)
	if err != nil || !due(sr.External, e.lib.clock.Now()) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, enrichTimeout)
	defer cancel()
	if _, err := e.lookup(ctx, sr); err != nil {
		e.log.WarnContext(ctx, "metadata lookup failed", "series", id, "err", err)
	}
}

// Lookup searches the series now (the user asked), even with enrichment
// off and even after a removed link.
func (e *Enricher) Lookup(ctx context.Context, id string) (domain.Series, error) {
	sr, err := e.lib.Get(id)
	if err != nil {
		return domain.Series{}, err
	}
	return e.lookup(ctx, sr)
}

// Unlink removes a wrong match; the series is not looked up again
// automatically.
func (e *Enricher) Unlink(ctx context.Context, id string) (domain.Series, error) {
	now := e.lib.clock.Now()
	return e.lib.updateSeries(ctx, id, func(sr *domain.Series) error {
		sr.External = &domain.External{Ignored: true, CheckedAt: now}
		return nil
	})
}

func (e *Enricher) lookup(ctx context.Context, sr domain.Series) (domain.Series, error) {
	m, err := enrich.Lookup(ctx, e.finders, enrich.Query{Titles: append([]string{sr.Title}, sr.AltTitles...), Type: sr.Type})
	if err != nil {
		return domain.Series{}, fmt.Errorf("look up %s: %w", sr.ID, err)
	}
	now := e.lib.clock.Now()
	ext := &domain.External{CheckedAt: now}
	if m != nil {
		ext = &domain.External{
			Source: m.Source, ID: m.ID, URL: m.URL, Title: m.Title, Status: m.Status,
			Genres: m.Genres, Year: m.Year, Synopsis: m.Synopsis, CheckedAt: now,
		}
	}
	return e.lib.updateSeries(ctx, sr.ID, func(s *domain.Series) error {
		if s.External != nil {
			ext.Extra = s.External.Extra // keep fields a newer version wrote
		}
		s.External = ext
		return nil
	})
}

// due reports whether an automatic lookup should run now.
func due(ext *domain.External, now time.Time) bool {
	switch {
	case ext == nil:
		return true
	case ext.Ignored:
		return false
	case ext.Found():
		return now.Sub(ext.CheckedAt) >= enrichFoundEvery
	}
	return now.Sub(ext.CheckedAt) >= enrichMissEvery
}
