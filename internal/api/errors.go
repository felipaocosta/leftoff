package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/llm"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/library"
	"github.com/felipaocosta/leftoff/internal/scheduler"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// errorMapping translates known errors into HTTP responses (SPEC §7).
// Order matters: the first match wins.
var errorMapping = []struct {
	target error
	status int
	code   string
}{
	{domain.ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
	{domain.ErrNotFound, http.StatusNotFound, "not_found"},
	{domain.ErrDuplicate, http.StatusConflict, "duplicate"},
	{library.ErrDifferentSeries, http.StatusConflict, "different_series"},
	{domain.ErrUnsupportedSite, http.StatusUnprocessableEntity, "unsupported_site"},
	{extract.ErrNeedsBrowser, http.StatusUnprocessableEntity, "needs_browser"}, // before ErrIncomplete, which it may wrap
	{extract.ErrIncomplete, http.StatusUnprocessableEntity, "extraction_incomplete"},
	{extract.ErrChapterPage, http.StatusUnprocessableEntity, "chapter_page"},
	{fetch.ErrNoBrowser, http.StatusBadGateway, "no_browser"}, // a blocked page nobody could solve
	{domain.ErrBlocked, http.StatusBadGateway, "blocked"},
	{library.ErrSourceShrank, http.StatusBadGateway, "source_changed"},
	{fetch.ErrTooLarge, http.StatusBadGateway, "upstream_too_large"},
	{storage.ErrNewerSchema, http.StatusUnprocessableEntity, "newer_schema"},
	{storage.ErrNotImage, http.StatusBadRequest, "invalid_image"},
	{storage.ErrBadRecipeName, http.StatusBadRequest, "invalid_input"},
	{library.ErrLLMDisabled, http.StatusServiceUnavailable, "llm_disabled"},
	{llm.ErrNoRecipe, http.StatusUnprocessableEntity, "no_recipe"},
	{llm.ErrModel, http.StatusBadGateway, "llm_error"},
	{scheduler.ErrJobNotFound, http.StatusNotFound, "not_found"},
	{scheduler.ErrNotRunning, http.StatusServiceUnavailable, "unavailable"},
	{context.DeadlineExceeded, http.StatusGatewayTimeout, "timeout"},
	{fetch.ErrUnreachable, http.StatusBadGateway, "upstream_error"}, // after timeout: a timeout is also unreachable
}

// writeError maps err to a status and code. Unknown errors become a 500
// whose details are logged but not sent to the client.
func (s *server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	for _, m := range errorMapping {
		if errors.Is(err, m.target) {
			writeErrorCode(w, m.status, m.code, err.Error())
			return
		}
	}
	var se *fetch.StatusError
	if errors.As(err, &se) {
		writeErrorCode(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeErrorCode(w, http.StatusRequestEntityTooLarge, "too_large", "request body too large")
		return
	}
	s.log.ErrorContext(r.Context(), "internal error", "path", r.URL.Path, "err", err)
	writeErrorCode(w, http.StatusInternalServerError, "internal", "internal error")
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeErrorCode(w http.ResponseWriter, status int, code, message string) {
	var body errorBody
	body.Error.Code, body.Error.Message = code, message
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client may be gone; nothing left to do
}
