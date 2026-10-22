package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

const fixtures = "../../testdata/fixtures"

// fakeAPI answers searches from saved responses, chosen by the searched
// title, and GETs by URL; anything else is an empty result.
type fakeAPI struct {
	t        *testing.T
	searches map[string]string // searched title → fixture file
	pages    map[string]string // GET URL → fixture file
	fail     bool
	asked    []string
}

func (f *fakeAPI) Fetch(_ context.Context, req *fetch.Request) (*fetch.Response, error) {
	if f.fail {
		return nil, &fetch.StatusError{URL: req.URL, StatusCode: http.StatusTooManyRequests}
	}
	file := f.pages[req.URL]
	if req.Method == http.MethodPost {
		var body struct {
			Search    string            `json:"search"`
			Variables map[string]string `json:"variables"`
		}
		if err := json.Unmarshal(req.Body, &body); err != nil {
			f.t.Fatal(err)
		}
		term := body.Search + body.Variables["search"]
		f.asked = append(f.asked, term)
		file = f.searches[term]
	}
	data := []byte(`{"data":{"Page":{"media":[]}},"results":[]}`)
	if file != "" {
		var err error
		if data, err = testfixture.ReadFile(f.t, filepath.Join(fixtures, file)); err != nil {
			f.t.Fatal(err)
		}
	}
	u, _ := url.Parse(req.URL)
	return &fetch.Response{URL: u, StatusCode: http.StatusOK, Body: data}, nil
}

func TestAniList_FindsByTitleAndSynonym(t *testing.T) {
	api := &fakeAPI{t: t, searches: map[string]string{
		"Eleceed":                               "anilist.co/search-eleceed.json",
		"Hunter Academy's Strongest Battle God": "anilist.co/search-hunter-academys-strongest-battle-god.json", // searched with a straight quote
	}}
	a := NewAniList(api, "")
	m, err := a.Find(context.Background(), Query{Titles: []string{"Eleceed", "일렉시드"}, Type: domain.TypeManhwa})
	if err != nil || m == nil {
		t.Fatalf("eleceed: %+v, %v", m, err)
	}
	if m.Source != "anilist" || m.ID != "106929" || m.URL != "https://anilist.co/manga/106929" || m.Status != domain.StatusOngoing ||
		m.Year == nil || *m.Year != 2018 || strings.Join(m.Genres, ",") != "Action,Comedy,Supernatural" || m.Synopsis == "" ||
		strings.Contains(m.Synopsis, "<br>") {
		t.Errorf("eleceed = %+v", m)
	}
	// The site's title (with a curly apostrophe) is only an AniList synonym.
	m, err = a.Find(context.Background(), Query{Titles: []string{"Hunter Academy’s Strongest Battle God"}, Type: domain.TypeManhwa})
	if err != nil || m == nil || m.ID != "177058" || m.Status != domain.StatusCompleted || m.Title != "Hunter Academy's Invincible Martial God" {
		t.Errorf("hunter = %+v, %v", m, err)
	}
}

func TestAniList_RejectsWrongKindAndUnknownTitles(t *testing.T) {
	api := &fakeAPI{t: t, searches: map[string]string{"Eleceed": "anilist.co/search-eleceed.json"}}
	a := NewAniList(api, "")
	for name, q := range map[string]Query{
		"other kind":      {Titles: []string{"Eleceed"}, Type: domain.TypeManga}, // AniList: Korean
		"no exact title":  {Titles: []string{"Eleceed Season 2"}, Type: domain.TypeManhwa},
		"nothing in base": {Titles: []string{"How to Survive at the Academy"}},
	} {
		if m, err := a.Find(context.Background(), q); err != nil || m != nil {
			t.Errorf("%s: %+v, %v", name, m, err)
		}
	}
	sort.Strings(api.asked)
	if strings.Join(api.asked, "|") != "Eleceed|Eleceed Season 2|How to Survive at the Academy" {
		t.Errorf("searched %v", api.asked)
	}
}

func TestMangaUpdates_PicksTheManhwaNotTheNovel(t *testing.T) {
	api := &fakeAPI{t: t,
		searches: map[string]string{"The Extra's Academy Survival Guide": "mangaupdates.com/search-the-extras-academy-survival-guide.json"},
		pages:    map[string]string{"https://api.mangaupdates.com/v1/series/41254648118": "mangaupdates.com/series-41254648118.json"},
	}
	m, err := NewMangaUpdates(api, "").Find(context.Background(),
		Query{Titles: []string{"The Extra’s Academy Survival Guide"}, Type: domain.TypeManhwa})
	if err != nil || m == nil {
		t.Fatalf("%+v, %v", m, err)
	}
	if m.Source != "mangaupdates" || m.ID != "41254648118" || m.Status != domain.StatusOngoing || *m.Year != 2023 ||
		!strings.HasPrefix(m.URL, "https://www.mangaupdates.com/series/iy9ya7q/") || len(m.Genres) < 4 ||
		!strings.HasPrefix(m.Synopsis, "Ed Rothstaylor is a third-rate villain") {
		t.Errorf("match = %+v", m)
	}

	// Without the type, both the manhwa and the novel ("(Novel)" ignored)
	// have the title: ambiguous, so no match.
	m, err = NewMangaUpdates(api, "").Find(context.Background(), Query{Titles: []string{"The Extra’s Academy Survival Guide"}})
	if err != nil || m != nil {
		t.Errorf("ambiguous: %+v, %v", m, err)
	}
}

func TestMangaUpdates_MatchesTheAlternativeTitleHit(t *testing.T) {
	api := &fakeAPI{t: t,
		searches: map[string]string{"Surviving the Academy": "mangaupdates.com/search-surviving-the-academy.json"},
		pages:    map[string]string{"https://api.mangaupdates.com/v1/series/41254648118": "mangaupdates.com/series-41254648118.json"},
	}
	m, err := NewMangaUpdates(api, "").Find(context.Background(),
		Query{Titles: []string{"Unknown Title", "Surviving the Academy"}, Type: domain.TypeManhwa})
	if err != nil || m == nil || m.ID != "41254648118" {
		t.Errorf("%+v, %v", m, err)
	}
}

// stubFinder returns a fixed answer.
type stubFinder struct {
	name string
	m    *Match
	err  error
}

func (s stubFinder) Name() string                                { return s.name }
func (s stubFinder) Find(context.Context, Query) (*Match, error) { return s.m, s.err }

func TestLookup(t *testing.T) {
	found := &Match{ID: "1"}
	down := stubFinder{name: "a", err: errors.New("down")}
	tests := []struct {
		name    string
		finders []Finder
		want    *Match
		wantErr bool
	}{
		{"first match wins", []Finder{stubFinder{name: "a"}, stubFinder{name: "b", m: found}}, found, false},
		{"one failure is not an error", []Finder{down, stubFinder{name: "b"}}, nil, false},
		{"every finder failed", []Finder{down, down}, nil, true},
		{"no finders", nil, nil, false},
	}
	for _, tt := range tests {
		m, err := Lookup(context.Background(), tt.finders, Query{Titles: []string{"x"}})
		if m != tt.want || (err != nil) != tt.wantErr {
			t.Errorf("%s: %+v, %v", tt.name, m, err)
		}
	}
}

func TestFind_PropagatesFetchErrors(t *testing.T) {
	api := &fakeAPI{t: t, fail: true}
	q := Query{Titles: []string{"Eleceed"}}
	if _, err := NewAniList(api, "").Find(context.Background(), q); err == nil {
		t.Error("anilist: expected error")
	}
	if _, err := NewMangaUpdates(api, "").Find(context.Background(), q); err == nil {
		t.Error("mangaupdates: expected error")
	}
}

func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{
		"The Extra’s Academy Survival Guide (Novel)": "theextrasacademysurvivalguide",
		"Hunter Academy's Strongest Battle God":      "hunteracademysstrongestbattlegod",
		"일렉시드":                                       "일렉시드",
		"  ":                                         "",
	} {
		if got := titleKey(in); got != want {
			t.Errorf("titleKey(%q) = %q, want %q", in, got, want)
		}
	}
	got := cleanSynopsis("A<br><br>B &amp; <i>C</i> [b]D[/b] **E**\n\n\n\nF")
	if got != "A\n\nB & C D E\n\nF" { // <br><br> is a paragraph break
		t.Errorf("cleanSynopsis = %q", got)
	}
	if s := cleanSynopsis(strings.Repeat("x", 3000)); len([]rune(s)) != maxSynopsis {
		t.Errorf("long synopsis: %d", len([]rune(s)))
	}
	if muStatus("12 Chapters (Complete)\n**S2:** (Ongoing)") != domain.StatusCompleted || muStatus("") != domain.StatusUnknown ||
		muStatus("40 Chapters (Complete/Axed)\n\n S1: 40") != domain.StatusCancelled || muStatus("3 Volumes (Hiatus)") != domain.StatusHiatus {
		t.Error("muStatus")
	}
	if got := searchTitles(Query{Titles: []string{"A", "a!", "B", "C", "D"}}); fmt.Sprint(got) != "[A B C]" {
		t.Errorf("searchTitles = %v", got)
	}
}
