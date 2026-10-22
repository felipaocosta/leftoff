package recipe

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

func read(t *testing.T, path string) []byte {
	t.Helper()
	b, err := testfixture.ReadFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParse_Valid(t *testing.T) {
	r, err := Parse(read(t, "../../../testdata/recipes/ravenscans.org.json"))
	if err != nil {
		t.Fatal(err)
	}
	if r.Domain != "ravenscans.org" || r.Method() != "recipe:ravenscans.org" || r.Chapters.ListURL != nil {
		t.Errorf("recipe = %+v", r)
	}
}

func TestParse_Invalid(t *testing.T) {
	base := `{"schema":1,"domain":"site.org","series":{"title":{"css":"h1"}},"chapters":{"item":{"css":"li"}}}`
	if _, err := Parse([]byte(base)); err != nil {
		t.Fatalf("minimal recipe should be valid: %v", err)
	}
	tests := map[string]string{
		"not json":         `{`,
		"wrong schema":     strings.Replace(base, `"schema":1`, `"schema":2`, 1),
		"bad domain":       strings.Replace(base, `"site.org"`, `"https://site.org/x"`, 1),
		"no title":         strings.Replace(base, `"title":{"css":"h1"}`, `"title":{"css":""}`, 1),
		"no chapter item":  strings.Replace(base, `"item":{"css":"li"}`, `"item":{}`, 1),
		"bad selector":     strings.Replace(base, `"css":"li"`, `"css":"li[["`, 1),
		"bad regex":        strings.Replace(base, `"item":{"css":"li"}`, `"item":{"css":"li"},"number_regex":"(("`, 1),
		"regex w/o group":  strings.Replace(base, `"item":{"css":"li"}`, `"item":{"css":"li"},"number_regex":"\\d+"`, 1),
		"bad alias domain": strings.Replace(base, `"domain":"site.org"`, `"domain":"site.org","aliases":["no dots"]`, 1),
	}
	for name, data := range tests {
		if _, err := Parse([]byte(data)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func page(t *testing.T, rawURL string, html []byte) *extract.Page {
	t.Helper()
	f := &fetch.Static{Pages: map[string][]byte{rawURL: html}}
	resp, err := fetch.Get(context.Background(), f, rawURL)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := goqueryDoc(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return &extract.Page{URL: resp.URL, Doc: doc}
}

var captured = domain.FixedClock{T: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}

func TestExtract_RavenscansRecipeMatchesTheAdapter(t *testing.T) {
	const u = "https://ravenscans.org/manga/the-extras-academy-survival-guide/"
	r, err := Parse(read(t, "../../../testdata/recipes/ravenscans.org.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := page(t, u, read(t, "../../../testdata/fixtures/ravenscans.org/the-extras-academy-survival-guide.html"))
	res, problems, err := Test(context.Background(), r, nil, p, captured)
	if err != nil || len(problems) > 0 {
		t.Fatalf("test: %v %v", err, problems)
	}
	s := res.Series
	if s.Title != "The Extra’s Academy Survival Guide" || s.StatusRaw != "Ongoing" || len(s.AltTitles) != 3 ||
		!strings.HasSuffix(s.CoverURL, "the-extras-academy-survival-guide.jpg") || s.Synopsis == "" {
		t.Errorf("series = %+v", s)
	}
	if len(res.Chapters) != 130 || *res.Chapters[0].Number != 128 || res.Chapters[0].ReleasedAt.String() != "2026-10-15" {
		t.Errorf("chapters = %d, first = %+v", len(res.Chapters), res.Chapters[0])
	}
}

func TestExtract_ListURLOnAnotherPage(t *testing.T) {
	const u = "https://www.mgeko.cc/manga/grand-finale/"
	r, err := Parse(read(t, "../../../testdata/recipes/mgeko.cc.json"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fetch.Static{Pages: map[string][]byte{
		u + "all-chapters/": read(t, "../../../testdata/fixtures/mgeko.cc/grand-finale-all-chapters.html"),
	}}
	p := page(t, u, read(t, "../../../testdata/fixtures/mgeko.cc/grand-finale.html"))
	res, problems, err := Test(context.Background(), r, f, p, captured)
	if err != nil || len(problems) > 0 {
		t.Fatalf("test: %v %v", err, problems)
	}
	if res.Series.Title != "Grand Finale" || len(res.Chapters) != 50 || *res.Chapters[0].Number != 50 ||
		strings.Contains(res.Series.CoverURL, "placeholder") {
		t.Errorf("series = %+v, chapters = %d", res.Series, len(res.Chapters))
	}
}

func TestTest_ReportsWhatIsMissing(t *testing.T) {
	r, err := Parse([]byte(`{"schema":1,"domain":"site.org","series":{"title":{"css":"h1.nope"}},"chapters":{"item":{"css":"li.nope"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	_, problems, err := Test(context.Background(), r, nil, page(t, "https://site.org/x", []byte(`<h1>T</h1>`)), captured)
	if err != nil || len(problems) != 2 {
		t.Errorf("problems = %v, err = %v", problems, err)
	}
}

func TestBook(t *testing.T) {
	b := NewBook(captured)
	errs := b.Load(map[string][]byte{
		"ravenscans.org": read(t, "../../../testdata/recipes/ravenscans.org.json"),
		"broken.org":     []byte(`{"schema":1}`),
	})
	if len(errs) != 1 || len(b.List()) != 1 {
		t.Errorf("errs = %v, list = %d", errs, len(b.List()))
	}
	for _, host := range []string{"ravenscans.org", "WWW.ravenscans.org", "ravenscans.com"} {
		if run, ok := b.Lookup(host); !ok || run.Method != "recipe:ravenscans.org" {
			t.Errorf("Lookup(%s) = %v %v", host, run.Method, ok)
		}
	}
	if _, ok := b.Lookup("other.org"); ok {
		t.Error("unknown host matched")
	}
	b.Load(nil)
	if _, ok := b.Lookup("ravenscans.org"); ok {
		t.Error("reload must replace the recipes")
	}
}

func goqueryDoc(b []byte) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(bytes.NewReader(b))
}
