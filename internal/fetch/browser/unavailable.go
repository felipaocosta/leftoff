//go:build !rod

package browser

import (
	"context"
	"errors"

	"github.com/felipaocosta/leftoff/internal/fetch"
)

// ErrUnavailable means this binary was built without browser support.
var ErrUnavailable = errors.New("browser solver not included in this build (build tag rod)")

// Solver is a placeholder used when the binary is built without "rod".
type Solver struct{}

// New returns the placeholder solver.
func New(Options) *Solver { return &Solver{} }

// Name implements fetch.Solver.
func (*Solver) Name() string { return "browser" }

// Solve implements fetch.Solver and always fails.
func (*Solver) Solve(context.Context, *fetch.Request) (*fetch.Solution, error) {
	return nil, ErrUnavailable
}

// Render implements extract.Renderer and always fails.
func (*Solver) Render(context.Context, string) (*fetch.Response, error) {
	return nil, ErrUnavailable
}

// Available reports whether this build can drive a browser.
func Available() bool { return false }
