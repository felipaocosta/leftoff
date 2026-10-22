package domain

import "errors"

// Domain sentinel errors. Lower layers wrap them with context
// (fmt.Errorf("...: %w", ErrX)) and the API maps them to HTTP status codes.
var (
	ErrNotFound        = errors.New("not found")
	ErrUnsupportedSite = errors.New("unsupported site")
	ErrBlocked         = errors.New("blocked by site")
	ErrDuplicate       = errors.New("already exists")
	ErrInvalidInput    = errors.New("invalid input")
)
