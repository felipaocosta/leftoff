package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/library"
)

type urlRequest struct {
	URL       string            `json:"url"`
	Overrides library.Overrides `json:"overrides"`
}

type readRequest struct {
	UpTo *float64 `json:"up_to"`
	Keys []string `json:"keys"`
	Read *bool    `json:"read"` // defaults to true
}

func (s *server) preview(w http.ResponseWriter, r *http.Request) {
	var req urlRequest
	if !s.decode(w, r, &req) {
		return
	}
	res, err := s.svc.Preview(r.Context(), req.URL)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		*extract.Result
		NeedsConfirmation bool `json:"needs_confirmation"`
	}{res, res.Confidence < extract.LowConfidence})
}

func (s *server) listSeries(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	found, err := s.svc.Find(q)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	views := make([]json.RawMessage, 0, len(found))
	for _, sr := range found {
		v, err := seriesView(sr, false)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		views = append(views, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": views})
}

// listSources returns the library's sites for the source filter (RF-33).
func (s *server) listSources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sources": s.svc.Sources()})
}

func (s *server) addSeries(w http.ResponseWriter, r *http.Request) {
	var req urlRequest
	if !s.decode(w, r, &req) {
		return
	}
	sr, err := s.svc.Add(r.Context(), req.URL, req.Overrides)
	s.respondSeries(w, r, http.StatusCreated, sr, err)
}

func (s *server) getSeries(w http.ResponseWriter, r *http.Request) {
	sr, err := s.svc.Get(r.PathValue("id"))
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

func (s *server) editSeries(w http.ResponseWriter, r *http.Request) {
	var p library.Patch
	if !s.decode(w, r, &p) {
		return
	}
	sr, err := s.svc.Edit(r.Context(), r.PathValue("id"), p)
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

func (s *server) removeSeries(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Remove(r.Context(), r.PathValue("id")); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) refreshSeries(w http.ResponseWriter, r *http.Request) {
	report, err := s.svc.Refresh(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, err) // the failure is also saved in check_error
		return
	}
	sr, err := s.svc.Get(report.SeriesID)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	view, err := seriesView(sr, true)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"new_chapters": report.NewChapters, "series": view})
}

func (s *server) markRead(w http.ResponseWriter, r *http.Request) {
	var req readRequest
	if !s.decode(w, r, &req) {
		return
	}
	id := r.PathValue("id")
	var (
		sr  domain.Series
		err error
	)
	switch {
	case req.UpTo != nil && len(req.Keys) > 0:
		err = fmt.Errorf("send either up_to or keys: %w", domain.ErrInvalidInput)
	case req.UpTo != nil:
		sr, err = s.svc.MarkReadUpTo(r.Context(), id, *req.UpTo)
	case len(req.Keys) > 0:
		sr, err = s.svc.MarkRead(r.Context(), id, req.Keys, req.Read == nil || *req.Read)
	default:
		err = fmt.Errorf("up_to or keys is required: %w", domain.ErrInvalidInput)
	}
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

func (s *server) markSeen(w http.ResponseWriter, r *http.Request) {
	sr, err := s.svc.MarkSeen(r.Context(), r.PathValue("id"))
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

func (s *server) cover(w http.ResponseWriter, r *http.Request) {
	data, contentType, err := s.covers.ReadCover(r.PathValue("id"))
	if errors.Is(err, os.ErrNotExist) {
		writeErrorCode(w, http.StatusNotFound, "not_found", "no local cover")
		return
	}
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	// Covers can be replaced (refresh, upload), so browsers revalidate them.
	etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data) // the client may be gone; nothing left to do
}

// maxCoverBytes limits uploaded covers.
const maxCoverBytes = 5 << 20

// changeSource moves a series to a new address (RF-08): {url, force}.
func (s *server) changeSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL   string `json:"url"`
		Force bool   `json:"force"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	sr, err := s.svc.ChangeSource(r.Context(), r.PathValue("id"), req.URL, req.Force)
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

func (s *server) refreshCover(w http.ResponseWriter, r *http.Request) {
	sr, err := s.svc.RefreshCover(r.Context(), r.PathValue("id"))
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

// uploadCover stores the raw request body (an image) as the series cover.
func (s *server) uploadCover(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCoverBytes))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	sr, err := s.svc.UploadCover(r.Context(), r.PathValue("id"), data)
	s.respondSeries(w, r, http.StatusOK, sr, err)
}

func (s *server) respondSeries(w http.ResponseWriter, r *http.Request, status int, sr domain.Series, err error) {
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	view, err := seriesView(sr, true)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, status, view)
}

// seriesView adds unread_count and has_new to a series. Lists omit the
// chapters (sent as null) to keep the payload small (RNF-03).
func seriesView(sr domain.Series, withChapters bool) (json.RawMessage, error) {
	extra := domain.Extra{
		"unread_count": json.RawMessage(strconv.Itoa(sr.UnreadCount())),
		"has_new":      json.RawMessage(strconv.FormatBool(sr.HasNew())),
	}
	if !withChapters {
		sr.Chapters = nil
	}
	return domain.MarshalWithExtra(sr, extra)
}

// decode reads a JSON body strictly (no unknown fields, size-limited) and
// writes the error response itself when it fails.
func (s *server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			s.writeError(w, r, err)
			return false
		}
		s.writeError(w, r, fmt.Errorf("decode body: %w: %w", err, domain.ErrInvalidInput))
		return false
	}
	return true
}

// parseQuery reads the GET /series filters (SPEC §7).
func parseQuery(r *http.Request) (library.Query, error) {
	v := r.URL.Query()
	q := library.Query{
		Text:     v.Get("q"),
		Status:   domain.Status(v.Get("status")),
		Category: domain.Category(v.Get("category")),
		Type:     domain.SeriesType(v.Get("type")),
		Source:   v.Get("source"),
		Sort:     v.Get("sort"),
	}
	if raw := v.Get("has_new"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return q, fmt.Errorf("has_new %q: %w", raw, domain.ErrInvalidInput)
		}
		q.HasNew = &b
	}
	return q, nil
}
