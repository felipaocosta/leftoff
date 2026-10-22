package app

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/heuristic"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Every smoke site with a golden file is a pipeline case, so the live
// check compares with what the offline tests guarantee.
func TestSmokeSites_MatchThePipelineCases(t *testing.T) {
	cases := map[string]pipelineCase{}
	for _, c := range pipelineCases() {
		cases[c.name] = c
	}
	for _, s := range smokeSites() {
		c, ok := cases[s.name]
		if !ok {
			continue // a site without fixtures (mangafire)
		}
		if c.method != s.method {
			t.Errorf("%s: method %q here, %q in the pipeline test", s.name, s.method, c.method)
		}
	}
}

func TestRunSmoke_ReportsEachVerdict(t *testing.T) {
	w := newWorld()
	grandFinale(t, w) // 49 chapters: one less than the golden, as if the site removed one
	w.set("https://www.mangaread.org/manga/eleceed/", fixture(t, "mangaread.org/eleceed.html"))
	clock := domain.FixedClock{T: time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)}
	client := fetch.New(fetch.Options{MinInterval: time.Millisecond, Transport: w, Retries: -1})
	ex := extract.New(client, extract.Options{Adapters: adapters(Config{}, clock), Fallback: heuristic.New(clock), Logger: slog.New(slog.DiscardHandler)})
	sites := []smokeSite{
		{"mangaread-eleceed", "https://www.mangaread.org/manga/eleceed/", "adapter:madara"},
		{"mgeko-grand-finale", gfURL, "adapter:mgeko"},
		{"down", "https://down.test/manga/x", "heuristic"},
		{"shell", "https://shell.test/title/x", "heuristic"},
	}
	w.set("https://shell.test/title/x", []byte(`<html><head><meta property="og:title" content="Shell"></head><body><div id="app"></div></body></html>`))
	var out strings.Builder
	err := runSmoke(context.Background(), ex, sites, "../../testdata/pipeline", &out)
	report := out.String()
	for _, want := range []string{
		"| mangaread-eleceed | ok | adapter:madara | Eleceed | 423 (423) | 423 (423) |",
		"| mgeko-grand-finale | changed | adapter:mgeko | Grand Finale | 49 (50) | 49 (50) |",
		"latest chapter went down",
		"| down | error |",
		"| shell | needs browser |",
		"4 sites, 2 need attention.",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	if err == nil {
		t.Error("a report with errors must fail the command")
	}
}
