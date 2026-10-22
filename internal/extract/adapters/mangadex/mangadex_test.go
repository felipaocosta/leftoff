package mangadex

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

var update = flag.Bool("update", false, "rewrite golden files")

const (
	fixtureDir = "../../../../testdata/fixtures/mangadex.org"
	fixtureID  = "b90aae00-1fc5-4b24-92ae-08dcbac1fc6d"
	pageURL    = "https://mangadex.org/title/" + fixtureID + "/koko-jidai-ni-gomandatta-jou-sama-to-no-dousei-seikatsu-wa-igaito-igokochi-ga-warukunai"
)

func fixtureFetcher(t *testing.T, a *Adapter) *fetch.Static {
	t.Helper()
	read := func(name string) []byte {
		b, err := testfixture.ReadFile(t, filepath.Join(fixtureDir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	return &fetch.Static{Pages: map[string][]byte{
		apiBase + "/manga/" + fixtureID + "?includes[]=cover_art": read("manga.json"),
		a.feedURL(fixtureID, 0):                                   read("feed-0.json"),
	}}
}

func TestAdapter_Fixture(t *testing.T) {
	a := New("pt-br", "en")
	f := fixtureFetcher(t, a)
	res, err := extract.New(f, extract.Options{Adapters: []extract.Adapter{a}}).Extract(context.Background(), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Series
	if res.Method != "adapter:mangadex" || s.Title != "Koko Jidai ni Gomandatta Jou sama to no Dousei Seikatsu wa Igaito Igokochi ga Warukunai" ||
		s.Status != domain.StatusOngoing || s.Type != domain.TypeManga || len(s.AltTitles) < 3 ||
		s.CoverURL != coverBase+"/"+fixtureID+"/0e706e8b-8788-4cca-973c-5ad149e309b7.jpg.512.jpg" || s.Synopsis == "" {
		t.Errorf("series = %+v", s)
	}
	if s.URL != "https://mangadex.org/title/"+fixtureID {
		t.Errorf("canonical url = %q", s.URL)
	}

	// Fixture: 49 distinct numbers; 45 exist in pt-br and en, 17.5, 36.5
	// and 45 only in en, 26.5 only in pt-br.
	if len(res.Chapters) != 49 {
		t.Errorf("chapters = %d, want 49 (deduplicated across languages)", len(res.Chapters))
	}
	if res.Chapters[0].Number == nil || *res.Chapters[0].Number != 45 {
		t.Fatalf("first chapter = %+v", res.Chapters[0])
	}
	withAlt := 0
	for _, c := range res.Chapters {
		if len(c.AltURLs) > 0 {
			withAlt++
		}
	}
	if withAlt < 45 {
		t.Errorf("%d chapters have an alt url, want at least the 45 found in both languages", withAlt)
	}
	if len(f.Requests) != 2 {
		t.Errorf("requests = %v (the page itself is never downloaded)", f.Requests)
	}
	checkGolden(t, filepath.Join(fixtureDir, "koko-jidai.golden.json"), res)
}

func TestAdapter_Match(t *testing.T) {
	a := New()
	for raw, want := range map[string]bool{
		pageURL: true,
		"https://mangadex.org/title/" + fixtureID:           true,
		"https://www.mangadex.org/title/" + fixtureID + "/": true,
		"https://mangadex.org/chapter/" + fixtureID:         false,
		"https://mangadex.org/title/not-a-uuid":             false,
		"https://other.org/title/" + fixtureID:              false,
	} {
		u, _ := url.Parse(raw)
		if got := a.Match(u, nil); got != want {
			t.Errorf("Match(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestToChapterInfo(t *testing.T) {
	var c chapterData
	c.ID = "abc"
	c.Attributes.Chapter = "10.5"
	c.Attributes.Title = " The Return "
	c.Attributes.PublishAt = "2026-10-11T23:30:00+09:00"
	got := toChapterInfo(c)
	if *got.Number != 10.5 || got.Label != "Chapter 10.5: The Return" || got.URL != siteBase+"/chapter/abc" || got.ReleasedAt.String() != "2026-10-11" {
		t.Errorf("chapter = %+v", got)
	}
	c.Attributes.Chapter, c.Attributes.Title, c.Attributes.ExternalURL = "", "", "https://official.example/1"
	got = toChapterInfo(c)
	if got.Number != nil || got.Label != "Oneshot" || got.URL != "https://official.example/1" {
		t.Errorf("oneshot/external = %+v", got)
	}
}

func TestTypeFor(t *testing.T) {
	for lang, want := range map[string]domain.SeriesType{"ja": domain.TypeManga, "ko": domain.TypeManhwa, "zh": domain.TypeManhua, "en": domain.TypeOther} {
		if got := typeFor(lang); got != want {
			t.Errorf("typeFor(%s) = %s", lang, got)
		}
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
