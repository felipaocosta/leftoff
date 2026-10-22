package library

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/enrich"
)

// fakeFinder answers every query with m (nil = not found) or err.
type fakeFinder struct {
	mu      sync.Mutex
	m       *enrich.Match
	err     error
	queries []enrich.Query
}

func (f *fakeFinder) Name() string { return "fake" }

func (f *fakeFinder) Find(_ context.Context, q enrich.Query) (*enrich.Match, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	return f.m, f.err
}

func newEnricher(t *testing.T, enabled bool, f *fakeFinder) (*env, *Enricher) {
	t.Helper()
	e := newEnv(t)
	settings := NewSettings(e.store, NotifyEnv{})
	if _, err := settings.Update(context.Background(), SettingsUpdate{Enrichment: &EnrichmentSettings{Enabled: enabled}}); err != nil {
		t.Fatal(err)
	}
	en := NewEnricher(e.svc, settings, []enrich.Finder{f}, nil)
	e.svc.UseEnricher(en)
	return e, en
}

var year = 2018

func TestEnricher_AddLooksUpWhenEnabled(t *testing.T) {
	f := &fakeFinder{m: &enrich.Match{Source: "anilist", ID: "106929", URL: "https://anilist.co/manga/106929",
		Title: "Eleceed", Status: domain.StatusOngoing, Genres: []string{"Action"}, Year: &year, Synopsis: "Kaiden…"}}
	e, _ := newEnricher(t, true, f)
	s, err := e.svc.Add(context.Background(), seriesURL, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	x := s.External
	if !x.Found() || x.ID != "106929" || x.Status != domain.StatusOngoing || *x.Year != 2018 || !x.CheckedAt.Equal(t0) {
		t.Errorf("external = %+v", x)
	}
	if q := f.queries[0]; q.Titles[0] != s.Title || q.Type != s.Type {
		t.Errorf("query = %+v", q)
	}

	// A refresh within 7 days does not look it up again; after, it does.
	e.clock.set(t0.Add(24 * time.Hour))
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	e.clock.set(t0.Add(8 * 24 * time.Hour))
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.queries) != 2 {
		t.Errorf("lookups = %d, want 2", len(f.queries))
	}
}

func TestEnricher_OffByDefaultButManualLookupWorks(t *testing.T) {
	f := &fakeFinder{} // nothing found
	e, en := newEnricher(t, false, f)
	s, err := e.svc.Add(context.Background(), seriesURL, Overrides{})
	if err != nil || s.External != nil || len(f.queries) != 0 {
		t.Fatalf("disabled add: external %+v, %d lookups, %v", s.External, len(f.queries), err)
	}
	s, err = en.Lookup(context.Background(), s.ID)
	if err != nil || s.External == nil || s.External.Found() || !s.External.CheckedAt.Equal(t0) {
		t.Errorf("manual miss: %+v, %v", s.External, err)
	}
}

func TestEnricher_UnlinkStopsAutomaticLookups(t *testing.T) {
	f := &fakeFinder{m: &enrich.Match{Source: "anilist", ID: "1"}}
	e, en := newEnricher(t, true, f)
	s, _ := e.svc.Add(context.Background(), seriesURL, Overrides{})
	s, err := en.Unlink(context.Background(), s.ID)
	if err != nil || s.External.Found() || !s.External.Ignored {
		t.Fatalf("unlink: %+v, %v", s.External, err)
	}
	e.clock.set(t0.Add(365 * 24 * time.Hour))
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.queries) != 1 {
		t.Errorf("an unlinked series must not be looked up: %d lookups", len(f.queries))
	}
	if s, _ = en.Lookup(context.Background(), s.ID); !s.External.Found() {
		t.Error("asking again must link it")
	}
}

func TestEnricher_FailuresNeverFailTheAdd(t *testing.T) {
	f := &fakeFinder{err: errors.New("429")}
	e, en := newEnricher(t, true, f)
	s, err := e.svc.Add(context.Background(), seriesURL, Overrides{})
	if err != nil || s.External != nil {
		t.Errorf("add: %+v, %v", s.External, err)
	}
	if _, err := en.Lookup(context.Background(), s.ID); err == nil {
		t.Error("a manual lookup reports the failure")
	}
	if _, err := en.Lookup(context.Background(), "nope"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown series: %v", err)
	}
}

func TestDue(t *testing.T) {
	now := t0
	found := &domain.External{ID: "1", CheckedAt: now.Add(-6 * 24 * time.Hour)}
	old := &domain.External{ID: "1", CheckedAt: now.Add(-8 * 24 * time.Hour)}
	miss := &domain.External{CheckedAt: now.Add(-20 * 24 * time.Hour)}
	oldMiss := &domain.External{CheckedAt: now.Add(-31 * 24 * time.Hour)}
	ignored := &domain.External{Ignored: true}
	for _, tt := range []struct {
		ext  *domain.External
		want bool
	}{{nil, true}, {found, false}, {old, true}, {miss, false}, {oldMiss, true}, {ignored, false}} {
		if got := due(tt.ext, now); got != tt.want {
			t.Errorf("due(%+v) = %v", tt.ext, got)
		}
	}
}

func TestSettings_EnrichmentEnvironmentLocks(t *testing.T) {
	e := newEnv(t)
	on := true
	s := NewSettings(e.store, NotifyEnv{}).WithEnrichEnv(&on)
	v, err := s.Update(context.Background(), SettingsUpdate{Enrichment: &EnrichmentSettings{Enabled: false}})
	if err != nil || !v.Enrichment.Enabled || !v.Enrichment.Locked || e.store.Settings().Enrichment.Enabled {
		t.Errorf("view = %+v, stored = %+v, %v", v.Enrichment, e.store.Settings().Enrichment, err)
	}
	// Saving notifications alone keeps the enrichment setting.
	plain := NewSettings(e.store, NotifyEnv{})
	if _, err := plain.Update(context.Background(), SettingsUpdate{Enrichment: &EnrichmentSettings{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	if v, _ := plain.Update(context.Background(), SettingsUpdate{Notifications: &NotificationSettings{}}); !v.Enrichment.Enabled {
		t.Error("a missing section must be left as it is")
	}
}
