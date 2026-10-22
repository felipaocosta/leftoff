package library

import (
	"context"
	"errors"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
)

func TestMigrateSource(t *testing.T) {
	sr := domain.Series{Source: domain.Source{URL: "https://old.org/manga/x/", Mirrors: []string{"https://older.org/manga/x"}}}
	if migrateSource(&sr, "https://www.old.org/manga/x", "m") {
		t.Error("same canonical URL is not a move")
	}
	if !migrateSource(&sr, "https://new.org/manga/x/", "adapter:y") {
		t.Fatal("expected a move")
	}
	if sr.Source.URL != "https://new.org/manga/x/" || sr.Source.Domain != "new.org" || sr.Source.Method != "adapter:y" {
		t.Errorf("source = %+v", sr.Source)
	}
	if len(sr.Source.Mirrors) != 2 || sr.Source.Mirrors[0] != "https://old.org/manga/x/" {
		t.Errorf("mirrors = %v", sr.Source.Mirrors)
	}
	// Moving back to an old mirror removes it from the mirrors.
	migrateSource(&sr, "https://older.org/manga/x/", "")
	if sr.Source.URL != "https://older.org/manga/x/" || len(sr.Source.Mirrors) != 2 || sr.Source.Mirrors[0] != "https://new.org/manga/x/" {
		t.Errorf("after moving back: %+v", sr.Source)
	}
}

func TestChangeSource(t *testing.T) {
	ctx := context.Background()
	newURL := "https://site.net/manga/x"
	e := newEnv(t)
	s := e.add(t)
	if _, err := e.svc.MarkReadUpTo(ctx, s.ID, 1); err != nil {
		t.Fatal(err)
	}

	// Same work on a new domain: same title, chapters at the same paths.
	e.ex.set(newURL, info(3, newURL+"/chapter-3"), info(2, newURL+"/chapter-2"), info(1, newURL+"/chapter-1"))
	got, err := e.svc.ChangeSource(ctx, s.ID, newURL, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Source.URL != newURL || got.Source.Domain != "site.net" || len(got.Source.Mirrors) != 1 || got.Source.Mirrors[0] != seriesURL {
		t.Errorf("source = %+v", got.Source)
	}
	if *got.LastReadChapter != 1 || len(got.Chapters) != 3 {
		t.Errorf("reading state lost: last read %v, %d chapters", got.LastReadChapter, len(got.Chapters))
	}

	// Another work is refused unless forced.
	other := "https://another.org/manga/z"
	e.ex.set(other, info(9, other+"/c9"))
	e.ex.results[other].Series.Title = "Completely Different"
	if _, err := e.svc.ChangeSource(ctx, s.ID, other, false); !errors.Is(err, ErrDifferentSeries) {
		t.Errorf("different work: %v", err)
	}
	if got, err := e.svc.ChangeSource(ctx, s.ID, other, true); err != nil || got.Source.URL != other {
		t.Errorf("forced: %+v %v", got.Source, err)
	}

	// The address of another series in the library is a duplicate.
	y := "https://site.org/manga/y"
	e.ex.set(y, info(1, y+"/c1"))
	if _, err := e.svc.Add(ctx, y, Overrides{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ChangeSource(ctx, s.ID, y, true); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	if _, err := e.svc.ChangeSource(ctx, "missing", newURL, false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}

func TestRefresh_FollowsADomainMove(t *testing.T) {
	e := newEnv(t)
	s := e.add(t)
	moved := "https://site.net/manga/x"
	e.ex.set(seriesURL, info(2, moved+"/chapter-2"), info(1, moved+"/chapter-1"))
	e.ex.results[seriesURL].Series.URL = moved // the site redirected
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(s.ID)
	if got.Source.URL != moved || len(got.Source.Mirrors) != 1 || got.Source.Mirrors[0] != seriesURL {
		t.Errorf("source = %+v", got.Source)
	}
	if _, err := e.svc.Add(context.Background(), seriesURL, Overrides{}); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("the old address must still count as a duplicate (mirror): %v", err)
	}
}

func TestSameWork(t *testing.T) {
	sr := domain.Series{Title: "Solo Leveling", AltTitles: []string{"Na Honjaman Level Up"}, Source: domain.Source{URL: "https://a.org/manga/solo-leveling/"}}
	tests := []struct {
		name string
		res  domain.SeriesInfo
		want bool
	}{
		{"same title, other punctuation", domain.SeriesInfo{Title: "solo leveling!", URL: "https://b.org/x"}, true},
		{"alt title", domain.SeriesInfo{Title: "Na Honjaman Level-Up", URL: "https://b.org/x"}, true},
		{"same slug", domain.SeriesInfo{Title: "SL", URL: "https://b.org/series/solo-leveling"}, true},
		{"different", domain.SeriesInfo{Title: "Other", URL: "https://b.org/other"}, false},
	}
	for _, tt := range tests {
		if got := sameWork(sr, &extract.Result{Series: tt.res}); got != tt.want {
			t.Errorf("%s: sameWork = %v", tt.name, got)
		}
	}
}
