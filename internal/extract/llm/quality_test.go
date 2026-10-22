package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

func TestNotes(t *testing.T) {
	n := 1.0
	dated := domain.ChapterInfo{Label: "Chapter 1", Number: &n, ReleasedAt: domain.NewDate(2026, 10, 1)}
	tests := []struct {
		name     string
		cover    string
		chapters []domain.ChapterInfo
		want     []string
	}{
		{"complete", "c.jpg", []domain.ChapterInfo{dated}, nil},
		{"no cover", "", []domain.ChapterInfo{dated}, []string{"series.cover"}},
		{"no dates", "c.jpg", []domain.ChapterInfo{{Label: "Chapter 1"}}, []string{"no chapter has a date"}},
		{"slug labels", "c.jpg", []domain.ChapterInfo{
			{Label: "50-eng-li", ReleasedAt: dated.ReleasedAt}, {Label: "49-eng-li", ReleasedAt: dated.ReleasedAt},
		}, []string{`URL parts (e.g. "49-eng-li")`}},
		{"few slugs", "c.jpg", []domain.ChapterInfo{dated, dated, {Label: "extra-1", ReleasedAt: dated.ReleasedAt}}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := notes(&extract.Result{Series: domain.SeriesInfo{CoverURL: tt.cover}, Chapters: tt.chapters}, &recipe.Recipe{})
			if len(got) != len(tt.want) {
				t.Fatalf("notes = %q, want %d", got, len(tt.want))
			}
			for i, w := range tt.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("note %q lacks %q", got[i], w)
				}
			}
		})
	}
}

func TestNotes_DateSelectorWithoutDates(t *testing.T) {
	r := &recipe.Recipe{}
	r.Chapters.Date = recipe.Field{CSS: ".chapter-update"}
	got := notes(&extract.Result{Series: domain.SeriesInfo{CoverURL: "c"}, Chapters: []domain.ChapterInfo{{Label: "Chapter 1"}}}, r)
	if len(got) != 1 || !strings.Contains(got[0], `".chapter-update" found no date`) || !strings.Contains(got[0], "datetime") {
		t.Errorf("notes = %q", got)
	}
}

func TestBetter(t *testing.T) {
	works := &Generated{Notes: []string{"a"}}
	perfect := &Generated{}
	fails := &Generated{Problems: []string{"x"}}
	failsMore := &Generated{Problems: []string{"x", "y"}}
	tests := []struct {
		name      string
		gen, best *Generated
		want      bool
	}{
		{"first", fails, nil, true},
		{"works beats fails", works, fails, true},
		{"fails never beats works", fails, works, false},
		{"fewer notes", perfect, works, true},
		{"same notes keeps the first", works, works, false},
		{"fewer problems", fails, failsMore, true},
		{"more problems", failsMore, fails, false},
	}
	for _, tt := range tests {
		if got := better(tt.gen, tt.best); got != tt.want {
			t.Errorf("%s: better = %v", tt.name, got)
		}
	}
}

// noDates is a ravenscans answer without the chapter date.
func noDates() string { return strings.Replace(answer("#chapterlist li", ""), ".chapterdate", "", 1) }

func TestGenerate_ImprovesAWorkingRecipe(t *testing.T) {
	m := &fakeModel{replies: []string{noDates(), answer("#chapterlist li", "")}}
	gen, err := NewGenerator(m, clock, 0).Generate(context.Background(), &fetch.Static{}, ravenPage(t), "ravenscans.org")
	if err != nil {
		t.Fatal(err)
	}
	if gen.Attempts != 2 || len(gen.Notes) != 0 || gen.Result.Chapters[0].ReleasedAt.IsZero() {
		t.Errorf("generated = %+v", gen)
	}
	if prompt := m.calls[1][3].Content; !strings.Contains(prompt, "That recipe works") || !strings.Contains(prompt, "no chapter has a date") {
		t.Errorf("improvement prompt = %q", prompt)
	}
}

func TestGenerate_KeepsTheWorkingRecipeWhenTheImprovementBreaksIt(t *testing.T) {
	m := &fakeModel{replies: []string{noDates(), answer("ul.nothing li", "")}}
	gen, err := NewGenerator(m, clock, 0).Generate(context.Background(), &fetch.Static{}, ravenPage(t), "ravenscans.org")
	if err != nil {
		t.Fatal(err)
	}
	if gen.Attempts != 1 || len(gen.Problems) != 0 || len(gen.Notes) != 1 || len(gen.Result.Chapters) != 130 {
		t.Errorf("generated = %+v", gen)
	}
}
