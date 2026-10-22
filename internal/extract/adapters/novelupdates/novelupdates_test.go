package novelupdates

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

var update = flag.Bool("update", false, "rewrite golden files")

const (
	fixtureDir = "../../../../testdata/fixtures/novelupdates.com"
	seriesURL  = "https://www.novelupdates.com/series/the-villainess-who-was-dumped-got-married-into-my-family-a-mob-noble-from-the-frontier-and-turns-out-shes-an-amazing-capable-wife-ln/"
)

func fixtures(t *testing.T) *fetch.Static {
	t.Helper()
	read := func(n string) []byte {
		b, err := testfixture.ReadFile(t, filepath.Join(fixtureDir, n))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	return &fetch.Static{Pages: map[string][]byte{
		seriesURL:           read("villainess-mob-noble.html"),
		seriesURL + "?pg=2": read("villainess-mob-noble-pg2.html"),
	}}
}

func TestAdapter_Fixture(t *testing.T) {
	f := fixtures(t)
	res, err := extract.New(f, extract.Options{Adapters: []extract.Adapter{New()}}).Extract(context.Background(), seriesURL)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Series
	if res.Method != "adapter:novelupdates" || !strings.HasPrefix(s.Title, "The Villainess Who Was Dumped") ||
		s.Type != domain.TypeNovel || s.StatusRaw != "4 Volumes" || len(s.AltTitles) != 1 ||
		!strings.HasPrefix(s.CoverURL, "https://cdn.novelupdates.com/images/") || s.Synopsis == "" {
		t.Errorf("series = %+v", s)
	}
	if len(res.Chapters) != 21 {
		t.Fatalf("chapters = %d, want 21 (15 on page 1, 6 on page 2)", len(res.Chapters))
	}
	first, last := res.Chapters[0], res.Chapters[20]
	if first.Label != "v2 interlude 1" || first.ReleasedAt.String() != "2026-09-11" ||
		last.Label != "v1c1 part1" || last.ReleasedAt.String() != "2026-06-10" {
		t.Errorf("first = %+v, last = %+v", first, last)
	}
	keys := map[string]bool{}
	for _, c := range res.Chapters {
		if c.URL != "" || c.Number != nil {
			t.Errorf("release without link must have no URL and (volume label) no number: %+v", c)
		}
		keys[c.Key()] = true
	}
	if len(keys) != 21 {
		t.Errorf("stable keys = %d, want 21 distinct", len(keys))
	}
	if strings.Join(f.Requests, " ") != seriesURL+" "+seriesURL+" "+seriesURL+"?pg=2" {
		t.Errorf("requests = %v", f.Requests)
	}
	checkGolden(t, filepath.Join(fixtureDir, "villainess-mob-noble.golden.json"), res)
}

func TestAdapter_Match(t *testing.T) {
	a := New()
	for raw, want := range map[string]bool{
		seriesURL:                           true,
		"https://novelupdates.com/series/x": true,
		"https://www.novelupdates.com/series/x/?pg=2": true,
		"https://www.novelupdates.com/group/x/":       false,
		"https://www.novelupdates.com/series/":        false,
		"https://other.org/series/x/":                 false,
	} {
		u, _ := url.Parse(raw)
		if got := a.Match(u, nil); got != want {
			t.Errorf("Match(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestRelease_PlainChaptersAndLinks(t *testing.T) {
	series, _ := url.Parse(seriesURL)
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<table id="myTable"><tbody>
		<tr><td>10/01/26</td><td><a href="/group/g/">G</a></td><td><a class="chp-release" href="//www.novelupdates.com/extnu/123/" title="c12">c12</a></td></tr>
		<tr><td>bad date</td><td>G</td><td><span>c12.5</span></td></tr>
		<tr><td>only</td><td>two</td></tr>
	</tbody></table>`))
	if err != nil {
		t.Fatal(err)
	}
	rows := doc.Find(selReleaseRow)
	c, ok := release(rows.Eq(0), series)
	if !ok || *c.Number != 12 || c.URL != "https://www.novelupdates.com/extnu/123/" || c.ReleasedAt.String() != "2026-10-01" {
		t.Errorf("linked release = %+v", c)
	}
	c, ok = release(rows.Eq(1), series)
	if !ok || *c.Number != 12.5 || c.URL != "" || !c.ReleasedAt.IsZero() {
		t.Errorf("plain release = %+v", c)
	}
	if _, ok := release(rows.Eq(2), series); ok {
		t.Error("rows without a label cell must be skipped")
	}
}

func TestPageURL(t *testing.T) {
	u, _ := url.Parse("https://www.novelupdates.com/series/x")
	if pageURL(u, 1) != "https://www.novelupdates.com/series/x/" || pageURL(u, 3) != "https://www.novelupdates.com/series/x/?pg=3" {
		t.Errorf("pageURL = %s, %s", pageURL(u, 1), pageURL(u, 3))
	}
}

func checkGolden(t *testing.T, path string, v any) {
	t.Helper()
	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("result differs from %s; run go test ./... -update and review the diff", path)
	}
}
