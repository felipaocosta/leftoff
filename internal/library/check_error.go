package library

import (
	"context"
	"errors"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// checkErrorCodes maps a refresh failure to its check_error.code, most
// specific first ("" when none matches).
var checkErrorCodes = []struct {
	target error
	code   string
}{
	{extract.ErrNeedsBrowser, domain.CheckNeedsBrowser}, // before ErrIncomplete, which it may wrap
	{fetch.ErrNoBrowser, domain.CheckNoBrowser},         // before ErrBlocked, which it wraps
	{domain.ErrBlocked, domain.CheckBlocked},
	{ErrSourceShrank, domain.CheckSourceChanged},
	{extract.ErrIncomplete, domain.CheckIncomplete},
	{domain.ErrNotFound, domain.CheckNotFound},
	{context.DeadlineExceeded, domain.CheckTimeout}, // before ErrUnreachable: a timeout is both
	{fetch.ErrUnreachable, domain.CheckUnreachable},
}

// checkErrorCode is the kind of a refresh failure, for the UI.
func checkErrorCode(err error) string {
	for _, m := range checkErrorCodes {
		if errors.Is(err, m.target) {
			return m.code
		}
	}
	var se *fetch.StatusError
	if errors.As(err, &se) {
		return domain.CheckUnreachable
	}
	return ""
}
