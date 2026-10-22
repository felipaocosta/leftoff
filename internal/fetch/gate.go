package fetch

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// hostGates enforces, per host, a single in-flight request and a minimum
// interval between the end of one request and the start of the next.
type hostGates struct {
	interval time.Duration
	clock    domain.Clock

	mu    sync.Mutex
	gates map[string]*gate
}

type gate struct {
	slot     chan struct{} // buffered(1): holding the token means "in flight"
	lastDone time.Time
}

func newHostGates(interval time.Duration, clock domain.Clock) *hostGates {
	return &hostGates{interval: interval, clock: clock, gates: map[string]*gate{}}
}

// acquire blocks until host may be requested and returns the release
// function, which must be called when the request is finished.
func (h *hostGates) acquire(ctx context.Context, host string) (func(), error) {
	g := h.gate(strings.ToLower(host))
	select {
	case g.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for %s: %w", host, ctx.Err())
	}
	if wait := g.lastDone.Add(h.interval).Sub(h.clock.Now()); wait > 0 {
		if err := sleep(ctx, wait); err != nil {
			<-g.slot
			return nil, fmt.Errorf("rate limit %s: %w", host, err)
		}
	}
	return func() {
		g.lastDone = h.clock.Now() // safe: only the token holder writes it
		<-g.slot
	}, nil
}

func (h *hostGates) gate(host string) *gate {
	h.mu.Lock()
	defer h.mu.Unlock()
	g, ok := h.gates[host]
	if !ok {
		g = &gate{slot: make(chan struct{}, 1)}
		h.gates[host] = g
	}
	return g
}

// sleep waits for d or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
