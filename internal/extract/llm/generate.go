package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// DefaultMaxBytes is how much cleaned HTML goes in the prompt (about 8k
// tokens); the model needs a context window of at least 12k tokens.
const DefaultMaxBytes = 24000

// attempts is the first answer plus one retry with the problems found
// (SPEC §5.5). A working recipe with notes gets one more answer to
// improve it.
const attempts = 2

// ErrNoRecipe means the model never produced a valid recipe.
var ErrNoRecipe = errors.New("the model did not produce a valid recipe")

// Completer is the chat model (implemented by *Client).
type Completer interface {
	Complete(ctx context.Context, msgs []Message, schema json.RawMessage) (string, error)
	Model() string
}

// Generator writes recipes with a Completer.
type Generator struct {
	llm      Completer
	clock    domain.Clock
	maxBytes int
}

// NewGenerator returns a generator; maxBytes <= 0 uses DefaultMaxBytes.
func NewGenerator(c Completer, clock domain.Clock, maxBytes int) *Generator {
	if clock == nil {
		clock = domain.SystemClock{}
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Generator{llm: c, clock: clock, maxBytes: maxBytes}
}

// Generated is a recipe written by the model and its test on the page.
// Problems is empty when the recipe worked; otherwise the recipe is valid
// but needs fixing by hand.
type Generated struct {
	Recipe   *recipe.Recipe  `json:"recipe"`
	Result   *extract.Result `json:"result"`
	Problems []string        `json:"problems"`
	Notes    []string        `json:"notes"` // what a working recipe could do better
	Attempts int             `json:"attempts"`
}

// Generate asks the model for a recipe for site (a host name) from the
// series page p, then tests it; f fetches a separate chapter list.
func (g *Generator) Generate(ctx context.Context, f fetch.Fetcher, p *extract.Page, site string) (*Generated, error) {
	cleaned, err := CleanHTML(p.Doc, g.maxBytes)
	if err != nil {
		return nil, err
	}
	msgs := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt(p.URL.String(), cleaned)},
	}
	var best *Generated
	sentList, improved := false, false
	for i := 1; ; i++ {
		reply, err := g.llm.Complete(ctx, msgs, draftSchema)
		if err != nil {
			return nil, err
		}
		gen, problems := g.check(ctx, f, p, site, reply)
		if gen != nil {
			gen.Attempts = i
			if better(gen, best) {
				best = gen
			}
			if len(problems) == 0 && len(gen.Notes) == 0 {
				return gen, nil
			}
		}
		working := best != nil && len(best.Problems) == 0
		if (working && improved) || (!working && i >= attempts) {
			break
		}
		improved = improved || working
		next := g.nextPrompt(ctx, f, p, best, gen, problems, &sentList)
		msgs = append(msgs, Message{Role: "assistant", Content: reply}, Message{Role: "user", Content: next})
	}
	if best == nil {
		return nil, ErrNoRecipe
	}
	return best, nil
}

// nextPrompt asks to fix the problems of a failed answer (with the
// separate chapter list page the first time it is needed) or to improve
// a working one.
func (g *Generator) nextPrompt(ctx context.Context, f fetch.Fetcher, p *extract.Page, best, gen *Generated, problems []string, sentList *bool) string {
	if best != nil && len(best.Problems) == 0 {
		return improvePrompt(best.Notes)
	}
	retry := retryPrompt(problems)
	if gen != nil && len(gen.Result.Chapters) == 0 && !*sentList {
		if list := g.listPage(ctx, f, p.URL, gen.Recipe); list != "" {
			retry, *sentList = retry+"\n\n"+list, true
		}
	}
	return retry
}

// check turns the reply into a recipe and tests it. It returns nil when
// the reply is not a valid recipe, and the problems to report either way.
func (g *Generator) check(ctx context.Context, f fetch.Fetcher, p *extract.Page, site, reply string) (*Generated, []string) {
	r, err := g.toRecipe(site, reply)
	if err != nil {
		return nil, []string{err.Error()}
	}
	if problem := foreignListURL(p.URL, r); problem != "" {
		return nil, []string{problem}
	}
	res, problems, err := recipe.Test(ctx, r, f, p, g.clock)
	if err != nil {
		return nil, []string{err.Error()}
	}
	gen := &Generated{Recipe: r, Result: res, Problems: append([]string{}, problems...), Notes: []string{}}
	if len(problems) == 0 {
		gen.Notes = notes(res, r)
	}
	return gen, problems
}

// listPage shows the model the separate chapter list page the recipe
// points to, whose HTML it has not seen; "" when there is none.
func (g *Generator) listPage(ctx context.Context, f fetch.Fetcher, page *url.URL, r *recipe.Recipe) string {
	if r.Chapters.ListURL == nil {
		return ""
	}
	base := *page
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/" // as the recipe engine resolves it
	}
	u, err := base.Parse(*r.Chapters.ListURL)
	if err != nil {
		return ""
	}
	resp, err := fetch.Get(ctx, f, u.String())
	if err != nil {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return ""
	}
	cleaned, err := CleanHTML(doc, g.maxBytes/2)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("The chapters are read from %s (list_url), not from the series page. "+
		"chapters.item, link, label and date must match THIS page:\n%s", u, cleaned)
}

// draft is the part of a recipe the model writes (draftSchema).
type draft struct {
	Series   json.RawMessage `json:"series"`
	Chapters struct {
		ListURL     string       `json:"list_url"`
		Item        recipe.Field `json:"item"`
		Link        recipe.Field `json:"link"`
		Label       recipe.Field `json:"label"`
		NumberRegex string       `json:"number_regex"`
		Date        recipe.Field `json:"date"`
	} `json:"chapters"`
}

func (g *Generator) toRecipe(site, reply string) (*recipe.Recipe, error) {
	var d draft
	if err := json.Unmarshal([]byte(stripFence(reply)), &d); err != nil {
		return nil, fmt.Errorf("the answer is not the recipe JSON: %w", err)
	}
	r := &recipe.Recipe{
		Schema: recipe.CurrentSchema, Domain: site,
		GeneratedBy: "llm:" + g.llm.Model(), UpdatedAt: g.clock.Now().UTC(),
	}
	if len(d.Series) > 0 {
		if err := json.Unmarshal(d.Series, &r.Series); err != nil {
			return nil, fmt.Errorf("series: %w", err)
		}
	}
	c := d.Chapters
	r.Chapters.Item, r.Chapters.Link, r.Chapters.Label, r.Chapters.Date = c.Item, c.Link, c.Label, c.Date
	r.Chapters.NumberRegex = strings.TrimSpace(c.NumberRegex)
	if l := strings.TrimSpace(c.ListURL); l != "" {
		r.Chapters.ListURL = &l
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}

// foreignListURL rejects a chapter list on another site: the recipe only
// reads the site it was written for.
func foreignListURL(page *url.URL, r *recipe.Recipe) string {
	if r.Chapters.ListURL == nil {
		return ""
	}
	u, err := page.Parse(*r.Chapters.ListURL)
	if err != nil || u.Host != page.Host {
		return fmt.Sprintf("chapters.list_url %q must be a page of %s", *r.Chapters.ListURL, page.Host)
	}
	return ""
}

// stripFence removes what some models add around the JSON despite the
// schema: a <think>…</think> block and a ```json fence.
func stripFence(s string) string {
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
		s = s[i+len("</think>"):]
	}
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = s[strings.IndexByte(s, '\n')+1:] // drops the line with the fence
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}
