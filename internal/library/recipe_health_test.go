package library

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
)

type seriesList []domain.Series

func (l seriesList) List() []domain.Series { return l }

func TestRecipes_ListReportsFailingSeries(t *testing.T) {
	r, _, _ := newRecipesEnv(t)
	data, err := testfixture.ReadFile(t, "../../testdata/recipes/ravenscans.org.json") // updated_at 2026-10-20, alias ravenscans.com
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Save(context.Background(), "ravenscans.org", data); err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	mk := func(id, domainName, method string, checked time.Time, failures int) domain.Series {
		s := domain.Series{ID: id, Title: "T" + id, AddedAt: before,
			Source: domain.Source{URL: "https://" + domainName + "/manga/" + id, Domain: domainName, Method: method}}
		s.LastCheckedAt = &checked
		if failures > 0 {
			s.CheckError = &domain.CheckError{Message: "boom", At: checked, Failures: failures}
		}
		return s
	}
	r.WithSeries(seriesList{
		mk("fallback", "ravenscans.org", "heuristic", after, 0),
		mk("browser-fallback", "www.ravenscans.com", "heuristic+browser", after, 0), // alias, with www
		mk("broken", "ravenscans.org", "recipe:ravenscans.org", after, 3),
		mk("two-failures", "ravenscans.org", "recipe:ravenscans.org", after, 2),
		mk("working", "ravenscans.org", "recipe:ravenscans.org", after, 0),
		mk("before-recipe", "ravenscans.org", "heuristic", before, 0),
		mk("adapter", "ravenscans.org", "adapter:mangastream", after, 5),
		mk("other-site", "site.org", "heuristic", after, 0),
	})
	list, err := r.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	var got []string
	for _, f := range list[0].Failing {
		got = append(got, f.SeriesID+":"+f.Reason)
		if f.SeriesID == "broken" && (f.Message != "boom" || f.URL != "https://ravenscans.org/manga/broken") {
			t.Errorf("broken = %+v", f)
		}
	}
	if want := "fallback:fallback,browser-fallback:fallback,broken:failing"; strings.Join(got, ",") != want {
		t.Errorf("failing = %v, want %s", got, want)
	}
}
