package llm

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
)

// notes lists what a working recipe misses that pages usually have. The
// model gets one chance to improve them; they are hints, not failures,
// because some sites really have no dates or cover.
func notes(res *extract.Result, r *recipe.Recipe) []string {
	var out []string
	if res.Series.CoverURL == "" {
		out = append(out, "series.cover found nothing (look for the cover <img> or meta og:image with attr \"content\")")
	}
	dated := 0
	for _, ch := range res.Chapters {
		if !ch.ReleasedAt.IsZero() {
			dated++
		}
	}
	if dated == 0 && len(res.Chapters) > 0 {
		out = append(out, dateNote(r.Chapters.Date))
	}
	if slug := slugLabel(res); slug != "" {
		out = append(out, fmt.Sprintf("chapter labels look like URL parts (e.g. %q): chapters.label should select "+
			"the element with the visible chapter name, such as \"Chapter 12\"", slug))
	}
	return out
}

// dateNote explains why no chapter got a date.
func dateNote(f recipe.Field) string {
	if f.CSS == "" {
		return "no chapter has a date: set chapters.date to the element with the release date " +
			"(e.g. \"October 15, 2026\", \"2 days ago\" or a <time datetime>) if the list shows one"
	}
	return fmt.Sprintf("chapters.date %q found no date the app understands: it reads absolute dates "+
		"(\"October 15, 2026\", \"Oct. 3, 2026\") and precise relative ones (\"3 days ago\"), not \"1 month\"; "+
		"if that element has a datetime or title attribute with the full date, set chapters.date.attr to it", f.CSS)
}

// slugLabel returns a label that looks like a URL slug ("50-eng-li"), or
// "" when most labels look like names.
func slugLabel(res *extract.Result) string {
	slugs, example := 0, ""
	for _, ch := range res.Chapters {
		l := ch.Label
		if l != "" && !strings.ContainsFunc(l, unicode.IsSpace) && strings.Contains(l, "-") {
			slugs++
			example = l
		}
	}
	if slugs*2 <= len(res.Chapters) {
		return ""
	}
	return example
}

// better reports whether gen should replace best: a recipe that works
// beats one that does not; among working ones, fewer notes win; among
// failing ones, fewer problems (or the latest on a tie).
func better(gen, best *Generated) bool {
	switch {
	case best == nil:
		return true
	case len(gen.Problems) == 0 && len(best.Problems) > 0:
		return true
	case len(gen.Problems) > 0 && len(best.Problems) == 0:
		return false
	case len(gen.Problems) == 0:
		return len(gen.Notes) < len(best.Notes)
	}
	return len(gen.Problems) <= len(best.Problems)
}

// improvePrompt asks for a better version of a working recipe.
func improvePrompt(notes []string) string {
	return "That recipe works. It could be better:\n- " + strings.Join(notes, "\n- ") +
		"\n\nLook at the HTML again. If the page has this data, answer with the whole improved recipe; " +
		"if it does not, answer with the same recipe."
}
