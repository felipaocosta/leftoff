// Package scheduler checks the library for new chapters periodically
// (RF-22, SPEC §9) and runs "refresh all" as background jobs (RF-21).
//
// Due series are grouped by site: up to Concurrency sites are checked in
// parallel and each site's series one after the other, so the per-site rate
// limit is respected. Series that keep failing are checked less often
// (backoff). It does NOT decide how a refresh works (library does).
package scheduler

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/library"
)

// Refresher is the part of *library.Service the scheduler uses.
type Refresher interface {
	List() []domain.Series
	Refresh(ctx context.Context, id string) (library.RefreshReport, error)
}

// Announcer sends the new chapters of a periodic check (implemented by
// *library.Announcer). It is called after every check, also without news,
// so it can resend what failed before.
type Announcer interface {
	Announce(ctx context.Context, reports []library.RefreshReport)
}

// Options configures the scheduler. Zero values select the SPEC defaults.
type Options struct {
	Interval       time.Duration // between checks of a series; default 6h (CHECK_INTERVAL_HOURS)
	Jitter         float64       // random spread of each wait, ±fraction; default 0.1
	Concurrency    int           // sites checked in parallel; default 4
	CheckCompleted bool          // also check completed/dropped series (RF-25); default false
	MaxBackoff     time.Duration // longest interval for failing series; default 7 days
	Announcer      Announcer     // optional: new-chapter notifications (RF-26)
	Clock          domain.Clock
	Logger         *slog.Logger
}

func (o Options) withDefaults() Options {
	if o.Interval <= 0 {
		o.Interval = 6 * time.Hour
	}
	if o.Jitter <= 0 {
		o.Jitter = 0.1
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 7 * 24 * time.Hour
	}
	if o.Clock == nil {
		o.Clock = domain.SystemClock{}
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	return o
}

// Summary is the outcome of one pass over the library.
type Summary struct {
	Checked     int `json:"checked"`
	Failed      int `json:"failed"`
	NewChapters int `json:"new_chapters"`
}

// Scheduler runs periodic checks. Create it with New and run it with Run.
type Scheduler struct {
	lib  Refresher
	opts Options
	mu   sync.Mutex // one pass at a time (periodic or manual)

	jobsMu sync.Mutex
	base   context.Context // set by Run; jobs run under it and stop with it
	jobs   map[string]*Job
	order  []string // job ids, oldest first
	seq    int
}

// New builds a Scheduler.
func New(lib Refresher, opts Options) *Scheduler {
	return &Scheduler{lib: lib, opts: opts.withDefaults(), jobs: map[string]*Job{}}
}

// Run checks due series now and then about every Interval/4 (with jitter)
// until ctx is canceled. Each series is still checked at most once per
// Interval; waking up more often just spreads the work.
func (s *Scheduler) Run(ctx context.Context) error {
	s.jobsMu.Lock()
	s.base = ctx
	s.jobsMu.Unlock()
	tick := s.opts.Interval / 4
	for {
		sum := s.CheckDue(ctx)
		if sum.Checked > 0 {
			s.opts.Logger.InfoContext(ctx, "scheduled check", "checked", sum.Checked,
				"failed", sum.Failed, "new_chapters", sum.NewChapters)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.jittered(tick)):
		}
	}
}

// Idle owns background jobs without periodic checks (checks disabled)
// until ctx is canceled.
func (s *Scheduler) Idle(ctx context.Context) error {
	s.jobsMu.Lock()
	s.base = ctx
	s.jobsMu.Unlock()
	<-ctx.Done()
	return ctx.Err()
}

// CheckDue refreshes the series that are due (see due) and hands the new
// chapters to the Announcer. Manual "refresh all" jobs do not notify: the
// user is looking at the screen.
func (s *Scheduler) CheckDue(ctx context.Context) Summary {
	now := s.opts.Clock.Now()
	var pending []domain.Series
	for _, sr := range s.lib.List() {
		if s.due(sr, now) {
			pending = append(pending, sr)
		}
	}
	var found []library.RefreshReport
	sum := s.refresh(ctx, pending, func(r library.RefreshReport, err error) {
		if err == nil && len(r.Added) > 0 {
			found = append(found, r) // refresh calls this under its lock
		}
	})
	if s.opts.Announcer != nil && ctx.Err() == nil {
		s.opts.Announcer.Announce(ctx, found)
	}
	return sum
}

// due reports whether a series should be checked now: not finished or
// dropped (unless CheckCompleted), and its interval elapsed. After three
// consecutive failures the interval doubles per extra failure, up to
// MaxBackoff (SPEC §9).
func (s *Scheduler) due(sr domain.Series, now time.Time) bool {
	if !s.opts.CheckCompleted && finished(sr) {
		return false
	}
	if sr.LastCheckedAt == nil {
		return true
	}
	return now.Sub(*sr.LastCheckedAt) >= s.interval(sr)
}

func (s *Scheduler) interval(sr domain.Series) time.Duration {
	iv := s.opts.Interval
	if sr.CheckError != nil {
		for f := sr.CheckError.Failures; f >= 3 && iv < s.opts.MaxBackoff; f-- {
			iv *= 2
		}
	}
	return min(iv, s.opts.MaxBackoff)
}

// finished: completed or cancelled works, and series the user finished or
// dropped, gain nothing from periodic checks (RF-25).
func finished(sr domain.Series) bool {
	switch {
	case sr.Status == domain.StatusCompleted, sr.Status == domain.StatusCancelled,
		sr.Category == domain.CategoryCompleted, sr.Category == domain.CategoryDropped:
		return true
	}
	return false
}

// refresh checks series grouped by site: Concurrency sites in parallel,
// one series at a time per site. progress, if set, is called after each.
func (s *Scheduler) refresh(ctx context.Context, series []domain.Series, progress func(library.RefreshReport, error)) Summary {
	s.mu.Lock()
	defer s.mu.Unlock()

	bySite := map[string][]string{}
	for _, sr := range series {
		bySite[sr.Source.Domain] = append(bySite[sr.Source.Domain], sr.ID)
	}
	sites := make([]string, 0, len(bySite))
	for d := range bySite {
		sites = append(sites, d)
	}
	sort.Strings(sites) // deterministic order

	var (
		mu  sync.Mutex
		sum Summary
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.opts.Concurrency)
	for _, site := range sites {
		g.Go(func() error {
			for _, id := range bySite[site] {
				if gctx.Err() != nil {
					return nil //nolint:nilerr // cancellation just stops the pass
				}
				report, err := s.lib.Refresh(gctx, id)
				mu.Lock()
				sum.Checked++
				sum.NewChapters += report.NewChapters
				if err != nil {
					sum.Failed++
				}
				if progress != nil {
					progress(report, err)
				}
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait() // goroutines never return errors; failures are counted
	return sum
}

// jittered spreads d by ±Jitter so checks do not hit all sites at once.
func (s *Scheduler) jittered(d time.Duration) time.Duration {
	spread := float64(d) * s.opts.Jitter
	return d + time.Duration((rand.Float64()*2-1)*spread) //nolint:gosec // timing jitter, not security
}
