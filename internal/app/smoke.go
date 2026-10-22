package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// smokeSite is a reference site (SPEC §14) checked by SmokeCommand. Name
// matches testdata/pipeline/<name>.golden.json when the site has one.
type smokeSite struct {
	name, url, method string
}

// smokeSites lists the SPEC §14 URLs, in the same order as there.
func smokeSites() []smokeSite {
	return []smokeSite{
		{"manhuaplus-forging", "https://manhuaplus.org/manga/forging-an-industrial-empire-in-a-magical-world", "adapter:liliana"},
		{"manhuaplus-necromancer", "https://manhuaplus.org/manga/catastrophic-necromancer001", "adapter:liliana"},
		{"mgeko-grand-finale", "https://www.mgeko.cc/manga/grand-finale/", "adapter:mgeko"},
		{"mgeko-yuumei-v-tuber-no-ani-dakedo-nazeka-ore-ga-yuumei-ni-natteita",
			"https://www.mgeko.cc/manga/yuumei-v-tuber-no-ani-dakedo-nazeka-ore-ga-yuumei-ni-natteita/", "adapter:mgeko"},
		{"novelupdates", "https://www.novelupdates.com/series/the-villainess-who-was-dumped-got-married-into-my-family-a-mob-noble-from-the-frontier-and-turns-out-shes-an-amazing-capable-wife-ln/", "adapter:novelupdates"},
		{"regressingknight-chapter-link", "https://w12.regressingknight.com/season-1-afterword-part-3/", "heuristic"},
		{"comix", "https://comix.to/title/zdk1m-sonna-kazoku-nara-sutechaeba", "heuristic"},
		{"mangaread-eleceed", "https://www.mangaread.org/manga/eleceed/", "adapter:madara"},
		{"ravenscans", "https://ravenscans.org/manga/the-extras-academy-survival-guide/", "adapter:mangastream"},
		{"mangadex", "https://mangadex.org/title/b90aae00-1fc5-4b24-92ae-08dcbac1fc6d/koko-jidai", "adapter:mangadex"},
		{"vortexscans", "https://vortexscans.org/series/survival-supremacy", "heuristic"},
		{"roliascan", "https://roliascan.com/manga/healing-at-the-academy-convenience-store/", "heuristic"},
		{"mangafire", "https://mangafire.to/title/rojly-gendai-dungeon-life-no-tsuzuki-wa-isekai-open-world-de", "heuristic"},
	}
}

// Smoke verdicts.
const (
	smokeOK      = "ok"
	smokeChanged = "changed" // the site no longer matches its saved fixture
	smokeBlocked = "blocked" // needs FlareSolverr or a local browser
	// smokeNeedsBrowser: a JavaScript page read without rendering (no
	// chapters, or only a few); a local browser or FlareSolverr reads it.
	smokeNeedsBrowser = "needs browser"
	smokeError        = "error"
)

type smokeRow struct {
	site     smokeSite
	verdict  string
	method   string
	title    string
	chapters string // now (fixture)
	latest   string // now (fixture)
	took     time.Duration
	notes    []string
}

// SmokeCommand checks every reference site against the live web (it makes
// real requests, at the configured rate limit) and prints a Markdown
// report comparing each result with testdata/pipeline when goldenDir has
// it. It is a manual tool, never part of go test (RULES §1.4). It fails
// when a site errors or changed; blocked sites only need a solver.
func SmokeCommand(ctx context.Context, cfg Config, logger *slog.Logger, goldenDir string, out io.Writer) error {
	_, ex, _ := newExtractor(cfg, logger, nil, nil, nil)
	return runSmoke(ctx, ex, smokeSites(), goldenDir, out)
}

func runSmoke(ctx context.Context, ex *extract.Extractor, sites []smokeSite, goldenDir string, out io.Writer) error {
	var rows []smokeRow
	for _, site := range sites {
		rows = append(rows, smokeOne(ctx, ex, site, goldenDir))
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return writeSmoke(out, rows)
}

func smokeOne(ctx context.Context, ex *extract.Extractor, site smokeSite, goldenDir string) smokeRow {
	row := smokeRow{site: site}
	start := time.Now()
	res, err := ex.Extract(ctx, site.url)
	row.took = time.Since(start).Round(100 * time.Millisecond)
	if err != nil {
		row.verdict = smokeError
		switch {
		case errors.Is(err, domain.ErrBlocked) || errors.Is(err, fetch.ErrNoBrowser):
			row.verdict = smokeBlocked
		case errors.Is(err, extract.ErrNeedsBrowser),
			errors.Is(err, extract.ErrIncomplete) && site.method == "heuristic":
			row.verdict = smokeNeedsBrowser
		}
		row.notes = append(row.notes, shorten(err.Error()))
		return row
	}
	row.verdict, row.method, row.title = smokeOK, res.Method, res.Series.Title
	now := summarize(res)
	row.chapters, row.latest = fmt.Sprint(now.count), now.latestText()
	if !strings.HasPrefix(res.Method, site.method) {
		row.verdict = smokeChanged
		row.notes = append(row.notes, "expected "+site.method)
	}
	if now.count < 3 && strings.HasPrefix(res.Method, "heuristic") && !strings.HasSuffix(res.Method, extract.RenderedSuffix) {
		row.verdict = smokeNeedsBrowser
		row.notes = append(row.notes, "few chapters without rendering")
		return row
	}
	golden, ok := readSmokeGolden(goldenDir, site.name)
	if !ok {
		return row
	}
	was := summarize(golden)
	row.chapters += fmt.Sprintf(" (%d)", was.count)
	row.latest += " (" + was.latestText() + ")"
	if res.Series.Title != golden.Series.Title {
		row.verdict = smokeChanged
		row.notes = append(row.notes, fmt.Sprintf("title was %q", golden.Series.Title))
	}
	if now.latest < was.latest {
		row.verdict = smokeChanged
		row.notes = append(row.notes, "latest chapter went down")
	}
	return row
}

type chapterSummary struct {
	count  int
	latest float64
}

func (c chapterSummary) latestText() string {
	if c.latest < 0 {
		return "—"
	}
	return domain.ChapterKey(&c.latest, "")
}

func summarize(r *extract.Result) chapterSummary {
	s := chapterSummary{count: len(r.Chapters), latest: -1}
	for _, c := range r.Chapters {
		if c.Number != nil && *c.Number > s.latest {
			s.latest = *c.Number
		}
	}
	return s
}

func readSmokeGolden(dir, name string) (*extract.Result, bool) {
	if dir == "" {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(dir, name+".golden.json"))
	if err != nil {
		return nil, false
	}
	var r extract.Result
	if json.Unmarshal(b, &r) != nil {
		return nil, false
	}
	return &r, true
}

func writeSmoke(out io.Writer, rows []smokeRow) error {
	var b strings.Builder
	b.WriteString("| Site | Result | Method | Title | Chapters now (fixture) | Latest now (fixture) | Time | Notes |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	failed := 0
	for _, r := range rows {
		if r.verdict == smokeError || r.verdict == smokeChanged {
			failed++
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", r.site.name, r.verdict, r.method,
			cell(r.title), r.chapters, r.latest, r.took, cell(strings.Join(r.notes, "; ")))
	}
	fmt.Fprintf(&b, "\n%d sites, %d need attention.\n", len(rows), failed)
	if _, err := io.WriteString(out, b.String()); err != nil {
		return fmt.Errorf("write smoke report: %w", err)
	}
	if failed > 0 {
		return fmt.Errorf("smoke: %d of %d sites need attention", failed, len(rows))
	}
	return nil
}

func cell(s string) string { return strings.ReplaceAll(s, "|", "\\|") }

func shorten(s string) string {
	if len(s) > 160 {
		return s[:157] + "…"
	}
	return s
}
