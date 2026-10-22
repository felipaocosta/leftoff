package scheduler

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/library"
)

var now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) *time.Time { t := now.Add(-d); return &t }

// fakeLib records refreshes, tracks per-site and overall parallelism and
// fails the ids in fail.
type fakeLib struct {
	series []domain.Series
	fail   map[string]bool
	delay  time.Duration

	mu          sync.Mutex
	refreshed   []string
	perSite     map[string]int
	maxPerSite  int
	running     atomic.Int32
	maxParallel atomic.Int32
}

func (f *fakeLib) List() []domain.Series { return f.series }

func (f *fakeLib) Refresh(ctx context.Context, id string) (library.RefreshReport, error) {
	site := ""
	for _, s := range f.series {
		if s.ID == id {
			site = s.Source.Domain
		}
	}
	f.mu.Lock()
	if f.perSite == nil {
		f.perSite = map[string]int{}
	}
	f.perSite[site]++
	f.maxPerSite = max(f.maxPerSite, f.perSite[site])
	f.refreshed = append(f.refreshed, id)
	f.mu.Unlock()
	n := f.running.Add(1)
	for {
		m := f.maxParallel.Load()
		if n <= m || f.maxParallel.CompareAndSwap(m, n) {
			break
		}
	}
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
	}
	f.running.Add(-1)
	f.mu.Lock()
	f.perSite[site]--
	f.mu.Unlock()
	if f.fail[id] {
		return library.RefreshReport{SeriesID: id}, errors.New("boom")
	}
	return library.RefreshReport{SeriesID: id, NewChapters: 1, Added: []domain.Chapter{{Key: "1"}}}, nil
}

// fakeAnnouncer records what each periodic check announced.
type fakeAnnouncer struct {
	mu    sync.Mutex
	calls [][]string
}

func (f *fakeAnnouncer) Announce(_ context.Context, reports []library.RefreshReport) {
	ids := []string{}
	for _, r := range reports {
		ids = append(ids, r.SeriesID)
	}
	sort.Strings(ids)
	f.mu.Lock()
	f.calls = append(f.calls, ids)
	f.mu.Unlock()
}

func TestCheckDue_AnnouncesNewChapters(t *testing.T) {
	lib := &fakeLib{fail: map[string]bool{"b": true}}
	lib.series = []domain.Series{series("a", "x"), series("b", "y"), series("c", "y")}
	ann := &fakeAnnouncer{}
	s := New(lib, Options{Interval: time.Hour, Announcer: ann})
	s.CheckDue(context.Background())
	if len(ann.calls) != 1 || strings.Join(ann.calls[0], ",") != "a,c" {
		t.Fatalf("calls = %v (the failed series has nothing to announce)", ann.calls)
	}

	// Nothing is due now, but the announcer is still called to retry.
	lib.series = nil
	s.CheckDue(context.Background())
	if len(ann.calls) != 2 || len(ann.calls[1]) != 0 {
		t.Errorf("calls = %v", ann.calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.CheckDue(ctx)
	if len(ann.calls) != 2 {
		t.Error("a canceled check must not announce")
	}
}

func series(id, site string, mods ...func(*domain.Series)) domain.Series {
	s := domain.Series{ID: id, Source: domain.Source{Domain: site}, Status: domain.StatusOngoing, Category: domain.CategoryReading}
	for _, m := range mods {
		m(&s)
	}
	return s
}

func TestDue(t *testing.T) {
	s := New(nil, Options{Clock: domain.FixedClock{T: now}})
	failing := func(n int, last time.Duration) func(*domain.Series) {
		return func(sr *domain.Series) {
			sr.LastCheckedAt = ago(last)
			sr.CheckError = &domain.CheckError{Failures: n}
		}
	}
	tests := []struct {
		name string
		sr   domain.Series
		want bool
	}{
		{"never checked", series("a", "x"), true},
		{"checked 7h ago", series("a", "x", func(s *domain.Series) { s.LastCheckedAt = ago(7 * time.Hour) }), true},
		{"checked 1h ago", series("a", "x", func(s *domain.Series) { s.LastCheckedAt = ago(time.Hour) }), false},
		{"completed work", series("a", "x", func(s *domain.Series) { s.Status = domain.StatusCompleted }), false},
		{"dropped by the user", series("a", "x", func(s *domain.Series) { s.Category = domain.CategoryDropped }), false},
		{"2 failures: normal interval", series("a", "x", failing(2, 7*time.Hour)), true},
		{"3 failures: 12h interval, 7h ago", series("a", "x", failing(3, 7*time.Hour)), false},
		{"3 failures: 12h interval, 13h ago", series("a", "x", failing(3, 13*time.Hour)), true},
		{"many failures: capped at 7 days", series("a", "x", failing(30, 8*24*time.Hour)), true},
	}
	for _, tt := range tests {
		if got := s.due(tt.sr, now); got != tt.want {
			t.Errorf("%s: due = %v, want %v", tt.name, got, tt.want)
		}
	}
	all := New(nil, Options{CheckCompleted: true})
	if !all.due(series("a", "x", func(s *domain.Series) { s.Status = domain.StatusCompleted }), now) {
		t.Error("CheckCompleted should include finished works")
	}
}

func TestCheckDue_GroupsBySiteAndLimitsParallelism(t *testing.T) {
	lib := &fakeLib{delay: 20 * time.Millisecond, fail: map[string]bool{"b2": true}}
	for _, site := range []string{"a", "b", "c", "d", "e", "f"} {
		lib.series = append(lib.series, series(site+"1", site+".org"), series(site+"2", site+".org"))
	}
	lib.series = append(lib.series, series("skip", "a.org", func(s *domain.Series) { s.LastCheckedAt = ago(time.Minute) }))
	s := New(lib, Options{Clock: domain.FixedClock{T: now}, Concurrency: 4})
	sum := s.CheckDue(context.Background())
	if sum != (Summary{Checked: 12, Failed: 1, NewChapters: 11}) {
		t.Errorf("summary = %+v", sum)
	}
	if lib.maxPerSite != 1 {
		t.Errorf("a site got %d parallel refreshes", lib.maxPerSite)
	}
	if p := lib.maxParallel.Load(); p > 4 || p < 2 {
		t.Errorf("max parallel = %d, want 2..4", p)
	}
	for _, id := range lib.refreshed {
		if id == "skip" {
			t.Error("a series checked a minute ago must be skipped")
		}
	}
}

func TestRun_ChecksAndStopsWithTheContext(t *testing.T) {
	lib := &fakeLib{series: []domain.Series{series("a", "x")}}
	s := New(lib, Options{Interval: 40 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if err := s.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run = %v", err)
	}
	if len(lib.refreshed) < 2 {
		t.Errorf("refreshed %d times, want several periodic checks", len(lib.refreshed))
	}
}

func TestJobs(t *testing.T) {
	lib := &fakeLib{delay: 10 * time.Millisecond, fail: map[string]bool{"b": true}}
	lib.series = []domain.Series{series("a", "x"), series("b", "y"), series("c", "y", func(s *domain.Series) { s.Status = domain.StatusCompleted })}
	s := New(lib, Options{Interval: time.Hour})
	if _, err := s.StartRefreshAll(); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("before Run: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Run(ctx) }()
	time.Sleep(50 * time.Millisecond) // first periodic pass

	job, err := s.StartRefreshAll()
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.StartRefreshAll(); again.ID != job.ID {
		t.Error("a running job must be reused")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		j, err := s.JobByID(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if j.Status == JobDone {
			if j.Total != 3 || j.Done != 3 || j.Failed != 1 || j.NewChapters != 2 || j.FinishedAt == nil {
				t.Errorf("job = %+v (refresh all includes finished works)", j)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %+v", j)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := s.JobByID("nope"); !errors.Is(err, ErrJobNotFound) {
		t.Errorf("unknown job: %v", err)
	}
}

func TestPrune_KeepsTheLatestJobs(t *testing.T) {
	s := New(&fakeLib{}, Options{})
	for i := range maxJobs + 5 {
		id := string(rune('A' + i))
		s.jobs[id] = &Job{ID: id, Status: JobDone}
		s.order = append(s.order, id)
	}
	s.prune()
	if len(s.order) != maxJobs || len(s.jobs) != maxJobs || s.order[0] != string(rune('A'+5)) {
		t.Errorf("kept %d jobs starting at %s", len(s.order), s.order[0])
	}
}
