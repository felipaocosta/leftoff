package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// fakeModel replies with the given answers in order and records prompts.
type fakeModel struct {
	replies []string
	calls   [][]Message
	err     error
}

func (m *fakeModel) Model() string { return "fake-7b" }

func (m *fakeModel) Complete(_ context.Context, msgs []Message, schema json.RawMessage) (string, error) {
	if !json.Valid(schema) {
		return "", errors.New("invalid schema")
	}
	m.calls = append(m.calls, append([]Message(nil), msgs...))
	if m.err != nil {
		return "", m.err
	}
	r := m.replies[0]
	m.replies = m.replies[1:]
	return r, nil
}

const ravenURL = "https://ravenscans.org/manga/the-extras-academy-survival-guide/"

func ravenPage(t *testing.T) *extract.Page {
	t.Helper()
	html, err := testfixture.ReadFile(t, "../../../testdata/fixtures/ravenscans.org/the-extras-academy-survival-guide.html")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(ravenURL)
	return &extract.Page{URL: u, Doc: parse(t, string(html))}
}

// answer is a model reply for ravenscans with the given chapter item.
func answer(item, listURL string) string {
	f := func(css, attr string) map[string]string {
		return map[string]string{"css": css, "attr": attr, "split": ""}
	}
	b, _ := json.Marshal(map[string]any{
		"series": map[string]any{
			"title": f("h1.entry-title", ""), "cover": f(".thumb img", "src"),
			"status": f(`.imptdt:contains("Status") i`, ""), "synopsis": f("", ""),
			"alt_titles": map[string]string{"css": ".alternative", "attr": "", "split": ","},
		},
		"chapters": map[string]any{
			"list_url": listURL, "item": f(item, ""), "link": f("a", "href"),
			"label": f(".chapternum", ""), "number_regex": "", "date": f(".chapterdate", ""),
		},
	})
	return string(b)
}

var clock = domain.FixedClock{T: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}

func TestGenerate_FirstAnswerWorks(t *testing.T) {
	m := &fakeModel{replies: []string{"```json\n" + answer("#chapterlist li", "") + "\n```"}}
	gen, err := NewGenerator(m, clock, 0).Generate(context.Background(), &fetch.Static{}, ravenPage(t), "ravenscans.org")
	if err != nil {
		t.Fatal(err)
	}
	r := gen.Recipe
	if gen.Attempts != 1 || len(gen.Problems) != 0 || r.Domain != "ravenscans.org" || r.GeneratedBy != "llm:fake-7b" ||
		!r.UpdatedAt.Equal(clock.T) || r.Chapters.ListURL != nil {
		t.Errorf("generated = %+v, recipe = %+v", gen, r)
	}
	if got := gen.Result; got.Series.Title != "The Extra’s Academy Survival Guide" || len(got.Chapters) != 130 {
		t.Errorf("result: %q, %d chapters", got.Series.Title, len(got.Chapters))
	}
	prompt := m.calls[0][1].Content
	if !strings.Contains(prompt, ravenURL) || !strings.Contains(prompt, `class="chapternum"`) || strings.Contains(prompt, "<script") {
		t.Errorf("prompt does not carry the cleaned page")
	}
}

func TestGenerate_RetriesWithTheProblems(t *testing.T) {
	m := &fakeModel{replies: []string{answer("ul.nothing li", ""), answer("#chapterlist li", "")}}
	gen, err := NewGenerator(m, clock, 0).Generate(context.Background(), &fetch.Static{}, ravenPage(t), "ravenscans.org")
	if err != nil {
		t.Fatal(err)
	}
	if gen.Attempts != 2 || len(gen.Problems) != 0 || len(gen.Result.Chapters) != 130 {
		t.Errorf("generated = %+v", gen)
	}
	retry := m.calls[1]
	if len(retry) != 4 || retry[2].Role != "assistant" || !strings.Contains(retry[3].Content, "chapters.item found no chapters") {
		t.Errorf("retry conversation = %+v", retry)
	}
}

func TestGenerate_ReturnsTheBestFailedRecipe(t *testing.T) {
	m := &fakeModel{replies: []string{answer("ul.nothing li", ""), `{"series":`}}
	gen, err := NewGenerator(m, clock, 0).Generate(context.Background(), &fetch.Static{}, ravenPage(t), "ravenscans.org")
	if err != nil {
		t.Fatal(err)
	}
	if gen.Attempts != 1 || len(gen.Problems) == 0 || gen.Recipe.Chapters.Item.CSS != "ul.nothing li" {
		t.Errorf("generated = %+v", gen)
	}
}

func TestGenerate_Fails(t *testing.T) {
	tests := []struct {
		name    string
		replies []string
		want    string
	}{
		{"not json", []string{"sorry", "still no"}, "not the recipe JSON"},
		{"invalid selector", []string{answer("li[[", ""), answer("li[[", "")}, "chapters.item.css"},
		{"foreign list url", []string{answer("li", "https://evil.example/list"), answer("li", "//evil.example/x")}, "must be a page of ravenscans.org"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &fakeModel{replies: tt.replies}
			_, err := NewGenerator(m, clock, 0).Generate(context.Background(), &fetch.Static{}, ravenPage(t), "ravenscans.org")
			if !errors.Is(err, ErrNoRecipe) {
				t.Fatalf("err = %v, want ErrNoRecipe", err)
			}
			if !strings.Contains(m.calls[1][3].Content, tt.want) {
				t.Errorf("retry prompt %q lacks %q", m.calls[1][3].Content, tt.want)
			}
		})
	}
}

func TestGenerate_ModelError(t *testing.T) {
	m := &fakeModel{err: ErrModel}
	_, err := NewGenerator(m, clock, 0).Generate(context.Background(), &fetch.Static{}, ravenPage(t), "ravenscans.org")
	if !errors.Is(err, ErrModel) {
		t.Errorf("err = %v, want ErrModel", err)
	}
}

func TestStripFence(t *testing.T) {
	for in, want := range map[string]string{
		`{"a":1}`:                         `{"a":1}`,
		"```json\n{\"a\":1}\n```":         `{"a":1}`,
		"<think>hmm</think>\n{\"a\":1}  ": `{"a":1}`,
	} {
		if got := stripFence(in); got != want {
			t.Errorf("stripFence(%q) = %q", in, got)
		}
	}
}

func TestGenerate_SendsTheSeparateChapterListOnRetry(t *testing.T) {
	const page = "https://www.mgeko.cc/manga/grand-finale/"
	series, _ := testfixture.ReadFile(t, "../../../testdata/fixtures/mgeko.cc/grand-finale.html")
	list, err := testfixture.ReadFile(t, "../../../testdata/fixtures/mgeko.cc/grand-finale-all-chapters.html")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(page)
	p := &extract.Page{URL: u, Doc: parse(t, string(series))}
	f := &fetch.Static{Pages: map[string][]byte{page + "all-chapters/": list}}

	wrong := strings.Replace(answer("li.wrong", "all-chapters/"), "h1.entry-title", "h1.novel-title", 1)
	right := strings.Replace(answer("ul.chapter-list li", "all-chapters/"), "h1.entry-title", "h1.novel-title", 1)
	// The working recipe has no dates, so the model gets one answer to
	// improve it; the same answer again keeps the first working one.
	m := &fakeModel{replies: []string{wrong, right, right}}
	gen, err := NewGenerator(m, clock, 0).Generate(context.Background(), f, p, "mgeko.cc")
	if err != nil {
		t.Fatal(err)
	}
	if gen.Attempts != 2 || len(gen.Result.Chapters) != 50 || len(m.calls) != 3 || len(gen.Notes) == 0 {
		t.Errorf("attempts %d, %d chapters, %d calls, notes %v", gen.Attempts, len(gen.Result.Chapters), len(m.calls), gen.Notes)
	}
	if improve := m.calls[2][5].Content; !strings.Contains(improve, "That recipe works") {
		t.Errorf("third prompt = %q", improve)
	}
	retry := m.calls[1][3].Content
	if !strings.Contains(retry, page+"all-chapters/ (list_url)") || !strings.Contains(retry, `class="chapter-list"`) {
		t.Errorf("retry prompt lacks the chapter list page:\n%.300s", retry)
	}
}
