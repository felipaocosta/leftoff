package library

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

func ptr[T any](v T) *T { return &v }

func TestEdit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := e.add(t)

	got, err := e.svc.Edit(ctx, s.ID, Patch{
		Title:     ptr("  New title "),
		AltTitles: ptr([]string{" A ", "", "A", "B"}),
		Type:      ptr(domain.TypeNovel),
		Tags:      ptr([]string{"fav"}),
		Notes:     ptr("great"),
		Category:  ptr(domain.CategoryOnHold),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "New title" || len(got.AltTitles) != 2 || got.Type != domain.TypeNovel ||
		got.Tags[0] != "fav" || got.Notes != "great" || got.Category != domain.CategoryOnHold {
		t.Errorf("edited = %+v", got)
	}
	untouched, _ := e.svc.Edit(ctx, s.ID, Patch{Notes: ptr("")})
	if untouched.Title != "New title" || untouched.Notes != "" {
		t.Errorf("nil fields must stay: %+v", untouched)
	}

	for name, p := range map[string]Patch{
		"empty title": {Title: ptr("  ")},
		"bad type":    {Type: ptr(domain.SeriesType("comic"))},
		"bad cat":     {Category: ptr(domain.Category("later"))},
	} {
		if _, err := e.svc.Edit(ctx, s.ID, p); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := e.svc.Edit(ctx, "missing", Patch{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing: err = %v", err)
	}
}

// seedLibrary adds three series with distinct titles, dates and state.
func seedLibrary(t *testing.T, e *env) (alpha, beta, gamma domain.Series) {
	t.Helper()
	ctx := context.Background()
	mk := func(slug, title string, added time.Time, chapters int) domain.Series {
		u := "https://site.org/manga/" + slug
		var chs []domain.ChapterInfo
		for i := chapters; i >= 1; i-- {
			chs = append(chs, info(float64(i), u+"/c"+string(rune('0'+i))))
		}
		e.ex.set(u, chs...)
		e.ex.results[u].Series.Title = title
		e.clock.set(added)
		s, err := e.svc.Add(ctx, u, Overrides{})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	alpha = mk("a", "Alpha", t0, 1)
	beta = mk("b", "beta Saga", t0.Add(time.Hour), 3)
	gamma = mk("g", "Gamma", t0.Add(2*time.Hour), 2)

	if _, err := e.svc.Edit(ctx, beta.ID, Patch{AltTitles: ptr([]string{"Second Story"}), Category: ptr(domain.CategoryOnHold)}); err != nil {
		t.Fatal(err)
	}
	// Gamma gets a new chapter: it is "new" and the most recently updated.
	e.clock.set(t0.Add(3 * time.Hour))
	u := "https://site.org/manga/g"
	e.ex.set(u, info(3, u+"/c3"), info(2, u+"/c2"), info(1, u+"/c1"))
	e.ex.results[u].Series.Title = "Gamma"
	if _, err := e.svc.Refresh(ctx, gamma.ID); err != nil {
		t.Fatal(err)
	}
	return alpha, beta, gamma
}

func TestFind(t *testing.T) {
	e := newEnv(t)
	alpha, beta, gamma := seedLibrary(t, e)
	tests := []struct {
		name string
		q    Query
		want []string
	}{
		{"default sort is most recently updated", Query{}, []string{gamma.ID, alpha.ID, beta.ID}},
		{"title", Query{Sort: SortTitle}, []string{alpha.ID, beta.ID, gamma.ID}},
		{"unread", Query{Sort: SortUnread}, []string{beta.ID, gamma.ID, alpha.ID}},
		{"added", Query{Sort: SortAdded}, []string{gamma.ID, beta.ID, alpha.ID}},
		{"text matches title case-insensitively", Query{Text: "SAGA"}, []string{beta.ID}},
		{"text matches alt titles", Query{Text: "second"}, []string{beta.ID}},
		{"category", Query{Category: domain.CategoryOnHold}, []string{beta.ID}},
		{"has new", Query{HasNew: ptr(true)}, []string{gamma.ID}},
		{"up to date", Query{HasNew: ptr(false), Sort: SortTitle}, []string{alpha.ID, beta.ID}},
		{"type and status", Query{Type: domain.TypeManhua, Status: domain.StatusOngoing, Sort: SortTitle}, []string{alpha.ID, beta.ID, gamma.ID}},
		{"no match", Query{Type: domain.TypeNovel}, []string{}},
		{"source", Query{Source: "WWW.Site.org ", Sort: SortTitle}, []string{alpha.ID, beta.ID, gamma.ID}},
		{"other source", Query{Source: "other.net"}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.svc.Find(tt.q)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, s := range got {
				ids = append(ids, s.ID)
			}
			if len(ids) != len(tt.want) {
				t.Fatalf("got %v, want %v", ids, tt.want)
			}
			for i := range ids {
				if ids[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", ids, tt.want)
				}
			}
		})
	}
	if got := e.svc.Sources(); len(got) != 1 || got[0] != (SourceCount{Domain: "site.org", Count: 3}) {
		t.Errorf("sources = %+v", got)
	}
	if _, err := e.svc.Find(Query{Sort: "random"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("bad sort: err = %v", err)
	}
}
