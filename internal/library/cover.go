package library

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// syncCover downloads the remote cover of a series and stores it locally
// (RF-41). It is best effort: covers are cosmetic, so failures are logged
// and never fail the add or refresh that triggered them.
func (s *Service) syncCover(ctx context.Context, id string) {
	if s.fetcher == nil {
		return
	}
	if err := s.downloadCover(ctx, id); err != nil {
		s.log.WarnContext(ctx, "cover download failed", "series", id, "err", err)
	}
}

func (s *Service) downloadCover(ctx context.Context, id string) error {
	series, err := s.Get(id)
	if err != nil {
		return err
	}
	remote := series.Cover.RemoteURL
	if remote == "" {
		return nil
	}
	resp, err := s.fetcher.Fetch(ctx, &fetch.Request{
		Method: http.MethodGet,
		URL:    remote,
		// Many scan sites block image hotlinking without a Referer.
		Header: http.Header{"Referer": {series.Source.URL}},
	})
	if err != nil {
		return fmt.Errorf("download cover %s: %w", remote, err)
	}
	rel, err := s.store.SaveCover(ctx, id, resp.Body)
	if err != nil {
		return err
	}
	_, err = s.updateSeries(ctx, id, func(sr *domain.Series) error {
		if sr.Cover.RemoteURL == remote && !sr.Cover.Custom { // ignore if the cover changed meanwhile
			now := s.clock.Now()
			sr.Cover.Local, sr.Cover.UpdatedAt = rel, &now
		}
		return nil
	})
	return err
}

// errNoSiteCover means the site page has no cover image to download.
var errNoSiteCover = errors.New("the site has no cover for this series")

// RefreshCover reads the series page again and downloads its current
// cover, even when the URL did not change; it replaces an uploaded image.
func (s *Service) RefreshCover(ctx context.Context, id string) (domain.Series, error) {
	current, err := s.Get(id)
	if err != nil {
		return domain.Series{}, err
	}
	if s.fetcher == nil {
		return domain.Series{}, fmt.Errorf("refresh cover %s: covers are disabled", id)
	}
	res, err := s.extractor.Extract(ctx, current.Source.URL)
	if err != nil {
		return domain.Series{}, fmt.Errorf("refresh cover %s: %w", id, err)
	}
	if res.Series.CoverURL == "" {
		return domain.Series{}, fmt.Errorf("refresh cover %s: %w: %w", id, errNoSiteCover, domain.ErrNotFound)
	}
	if _, err := s.updateSeries(ctx, id, func(sr *domain.Series) error {
		sr.Cover.RemoteURL, sr.Cover.Custom = res.Series.CoverURL, false
		return nil
	}); err != nil {
		return domain.Series{}, err
	}
	if err := s.downloadCover(ctx, id); err != nil {
		return domain.Series{}, err
	}
	return s.Get(id)
}

// UploadCover stores an image chosen by the user as the series cover
// (jpg, png, gif or webp). Refreshes keep it until RefreshCover is used.
func (s *Service) UploadCover(ctx context.Context, id string, data []byte) (domain.Series, error) {
	if _, err := s.Get(id); err != nil {
		return domain.Series{}, err
	}
	rel, err := s.store.SaveCover(ctx, id, data)
	if err != nil {
		return domain.Series{}, err
	}
	return s.updateSeries(ctx, id, func(sr *domain.Series) error {
		now := s.clock.Now()
		sr.Cover.Local, sr.Cover.Custom, sr.Cover.UpdatedAt = rel, true, &now
		return nil
	})
}
