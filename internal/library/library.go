// Package library holds the business rules of LeftOff: adding a
// series from a URL, refreshing it (merging chapters without losing read
// state), marking chapters as read and tracking what is new.
//
// It does NOT know HTTP (api), how pages are parsed (extract) or how data
// is written to disk (storage); those are injected.
package library

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"

	"github.com/oklog/ulid/v2"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// Extractor turns a URL into series data (implemented by *extract.Extractor).
type Extractor interface {
	Extract(ctx context.Context, rawURL string) (*extract.Result, error)
}

// Store persists the library and covers (implemented by *storage.Store).
type Store interface {
	Library() *storage.Library
	Update(ctx context.Context, fn func(*storage.Library) error) error
	SaveCover(ctx context.Context, id string, data []byte) (string, error)
	DeleteCover(ctx context.Context, id string) error
	Backup(ctx context.Context) error
}

// Options holds optional dependencies.
type Options struct {
	Fetcher fetch.Fetcher // downloads covers (RF-41); nil disables them
	Logger  *slog.Logger  // default discards
}

// Service implements the library use cases. It is safe for concurrent use:
// every change goes through Store.Update.
type Service struct {
	store     Store
	extractor Extractor
	clock     domain.Clock
	fetcher   fetch.Fetcher
	log       *slog.Logger
	entropy   io.Reader  // for ULIDs
	enricher  AfterSaver // optional (RF-57), see UseEnricher
}

// New builds a Service.
func New(store Store, extractor Extractor, clock domain.Clock, opts Options) *Service {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	return &Service{
		store: store, extractor: extractor, clock: clock,
		fetcher: opts.Fetcher, log: opts.Logger, entropy: rand.Reader,
	}
}

// Preview extracts a URL without saving anything (RF-02).
func (s *Service) Preview(ctx context.Context, rawURL string) (*extract.Result, error) {
	return s.extractor.Extract(ctx, rawURL)
}

// List returns all series from the current snapshot. The slice and its
// elements are shared and must not be modified.
func (s *Service) List() []domain.Series { return s.store.Library().Series }

// Get returns one series by id, or domain.ErrNotFound.
func (s *Service) Get(id string) (domain.Series, error) {
	lib := s.store.Library()
	i := lib.SeriesByID(id)
	if i < 0 {
		return domain.Series{}, fmt.Errorf("series %s: %w", id, domain.ErrNotFound)
	}
	return lib.Series[i], nil
}

// Remove deletes a series and its cover (RF-06; the UI asks for
// confirmation). A cover that cannot be deleted is only logged.
func (s *Service) Remove(ctx context.Context, id string) error {
	err := s.store.Update(ctx, func(lib *storage.Library) error {
		i, err := find(lib, id)
		if err != nil {
			return err
		}
		lib.Series = append(lib.Series[:i], lib.Series[i+1:]...)
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.store.DeleteCover(ctx, id); err != nil {
		s.log.WarnContext(ctx, "delete cover", "series", id, "err", err)
	}
	return nil
}

// updateSeries applies fn to the series with the given id inside one
// transactional store update and returns the saved series.
func (s *Service) updateSeries(ctx context.Context, id string, fn func(*domain.Series) error) (domain.Series, error) {
	err := s.store.Update(ctx, func(lib *storage.Library) error {
		i, err := find(lib, id)
		if err != nil {
			return err
		}
		return fn(&lib.Series[i])
	})
	if err != nil {
		return domain.Series{}, err
	}
	return s.Get(id)
}

func (s *Service) newID() (string, error) {
	id, err := ulid.New(ulid.Timestamp(s.clock.Now()), s.entropy)
	if err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return id.String(), nil
}

func find(lib *storage.Library, id string) (int, error) {
	i := lib.SeriesByID(id)
	if i < 0 {
		return 0, fmt.Errorf("series %s: %w", id, domain.ErrNotFound)
	}
	return i, nil
}
