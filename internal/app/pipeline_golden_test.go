package app

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/heuristic"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

var update = flag.Bool("update", false, "rewrite golden files")

const fixtures = "../../testdata/fixtures/"

// pipelineCase is a reference site (SPEC §14): the URL a user pastes, the
// saved pages the pipeline may ask for and the method that must win.
type pipelineCase struct {
	name, url, method string
	pages             map[string]string // URL → fixture file under testdata/fixtures
}

func mangadexFeed(id string) string {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(500))
	q.Set("offset", "0")
	q.Set("order[chapter]", "desc")
	for _, l := range []string{"pt-br", "en"} {
		q.Add("translatedLanguage[]", l)
	}
	for _, r := range []string{"safe", "suggestive", "erotica"} {
		q.Add("contentRating[]", r)
	}
	return "https://api.mangadex.org/manga/" + id + "/feed?" + q.Encode()
}

func pipelineCases() []pipelineCase {
	const md = "b90aae00-1fc5-4b24-92ae-08dcbac1fc6d"
	const nu = "https://www.novelupdates.com/series/the-villainess-who-was-dumped-got-married-into-my-family-a-mob-noble-from-the-frontier-and-turns-out-shes-an-amazing-capable-wife-ln/"
	mgeko := func(slug string) pipelineCase {
		base := "https://www.mgeko.cc/manga/" + slug + "/"
		return pipelineCase{"mgeko-" + slug, base, "adapter:mgeko", map[string]string{
			base: "mgeko.cc/" + slug + ".html", base + "all-chapters/": "mgeko.cc/" + slug + "-all-chapters.html",
		}}
	}
	single := func(name, rawURL, method, file string) pipelineCase {
		return pipelineCase{name, rawURL, method, map[string]string{rawURL: file}}
	}
	return []pipelineCase{
		single("manhuaplus-forging", "https://manhuaplus.org/manga/forging-an-industrial-empire-in-a-magical-world", "adapter:liliana",
			"manhuaplus.org/forging-an-industrial-empire-in-a-magical-world.html"),
		single("manhuaplus-necromancer", "https://manhuaplus.org/manga/catastrophic-necromancer001", "adapter:liliana",
			"manhuaplus.org/catastrophic-necromancer001.html"),
		mgeko("grand-finale"),
		mgeko("yuumei-v-tuber-no-ani-dakedo-nazeka-ore-ga-yuumei-ni-natteita"),
		{"novelupdates", nu, "adapter:novelupdates", map[string]string{
			nu: "novelupdates.com/villainess-mob-noble.html", nu + "?pg=2": "novelupdates.com/villainess-mob-noble-pg2.html",
		}},
		{"regressingknight-chapter-link", "https://w12.regressingknight.com/season-1-afterword-part-3/", "heuristic", map[string]string{
			"https://w12.regressingknight.com/season-1-afterword-part-3/": "regressingknight.com/season-1-afterword-part-3.html",
			"https://w12.regressingknight.com/":                           "regressingknight.com/home.html",
		}},
		single("comix", "https://comix.to/title/zdk1m-sonna-kazoku-nara-sutechaeba", "heuristic", "comix.to/sonna-kazoku-nara-sutechaeba.html"),
		single("mangaread-eleceed", "https://www.mangaread.org/manga/eleceed/", "adapter:madara", "mangaread.org/eleceed.html"),
		single("mangaread-hunter", "https://www.mangaread.org/manga/hunter-academys-strongest-battle-god/", "adapter:madara",
			"mangaread.org/hunter-academys-strongest-battle-god.html"),
		single("ravenscans", "https://ravenscans.org/manga/the-extras-academy-survival-guide/", "adapter:mangastream",
			"ravenscans.org/the-extras-academy-survival-guide.html"),
		{"mangadex", "https://mangadex.org/title/" + md + "/koko-jidai", "adapter:mangadex", map[string]string{
			"https://api.mangadex.org/manga/" + md + "?includes[]=cover_art": "mangadex.org/manga.json",
			mangadexFeed(md): "mangadex.org/feed-0.json",
		}},
		single("vortexscans", "https://vortexscans.org/series/survival-supremacy", "heuristic", "vortexscans.org/survival-supremacy.html"),
		single("roliascan", "https://roliascan.com/manga/healing-at-the-academy-convenience-store/", "heuristic",
			"roliascan.com/healing-at-the-academy-convenience-store.html"),
	}
}

// TestPipeline_ReferenceSites runs every reference site through the same
// pipeline as the app (adapter list, engine detection, heuristic) and
// compares the whole result with testdata/pipeline/<name>.golden.json, so
// a change in one extractor that alters another site shows up here.
func TestPipeline_ReferenceSites(t *testing.T) {
	clock := domain.FixedClock{T: time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)}
	cfg := Config{MangaDexLangs: []string{"pt-br", "en"}}
	for _, tc := range pipelineCases() {
		t.Run(tc.name, func(t *testing.T) {
			pages := map[string][]byte{}
			for u, file := range tc.pages {
				b, err := testfixture.ReadFile(t, fixtures+file)
				if err != nil {
					t.Fatal(err)
				}
				pages[u] = b
			}
			f := &fetch.Static{Pages: pages}
			ex := extract.New(f, extract.Options{Adapters: adapters(cfg, clock), Fallback: heuristic.New(clock)})
			res, err := ex.Extract(context.Background(), tc.url)
			if err != nil {
				t.Fatalf("extract: %v (requests %v)", err, f.Requests)
			}
			if res.Method != tc.method {
				t.Errorf("method = %q, want %q", res.Method, tc.method)
			}
			compareGolden(t, filepath.Join("..", "..", "testdata", "pipeline", tc.name+".golden.json"), res)
		})
	}
}

func compareGolden(t *testing.T, path string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/app/ -run TestPipeline -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("result differs from %s; run with -update and review the diff", path)
	}
}
