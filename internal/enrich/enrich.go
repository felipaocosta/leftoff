// Package enrich finds a series in metadata databases, AniList and
// MangaUpdates, for its official status, genres, year and synopsis
// (RF-57). Requests go through the injected fetch.Fetcher, so they are
// rate limited and cached like any other.
//
// Matching is strict on purpose: a candidate counts only when one of its
// titles equals one of the series' titles (ignoring case, punctuation and
// a trailing "(Novel)") and its kind (manga, manhwa, manhua, novel) does
// not contradict the series type. Several such candidates are ambiguous
// and give no match. A wrong link is worse than none.
//
// It does NOT store anything (library does).
package enrich

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// maxSynopsis caps the stored synopsis, in characters.
const maxSynopsis = 2000

// Query describes the series to look up.
type Query struct {
	Titles []string          // title first, then alternative titles
	Type   domain.SeriesType // domain.TypeOther when unknown
}

// Match is the entry found for a series.
type Match struct {
	Source   string // domain.ExternalAniList or domain.ExternalMangaUpdates
	ID       string
	URL      string
	Title    string
	Status   domain.Status
	Genres   []string
	Year     *int
	Synopsis string
}

// Finder searches one database. Find returns nil, nil when nothing matches.
type Finder interface {
	Name() string
	Find(ctx context.Context, q Query) (*Match, error)
}

// maxSearches is how many of the series' titles are searched per database.
const maxSearches = 3

// Lookup asks the finders in order and returns the first match; nil, nil
// when none matched. It fails only if every finder failed.
func Lookup(ctx context.Context, finders []Finder, q Query) (*Match, error) {
	var errs []error
	for _, f := range finders {
		m, err := f.Find(ctx, q)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.Name(), err))
			continue
		}
		if m != nil {
			return m, nil
		}
	}
	if len(errs) == len(finders) && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return nil, nil
}

// candidate is an entry from a search, before matching.
type candidate struct {
	titles []string
	kind   domain.SeriesType // TypeOther when unknown
	load   func(ctx context.Context) (*Match, error)
}

// pick returns the only candidate that matches q, or nil (no match, or
// ambiguous).
func pick(q Query, cands []candidate) *candidate {
	want := map[string]bool{}
	for _, t := range q.Titles {
		if k := titleKey(t); k != "" {
			want[k] = true
		}
	}
	var found *candidate
	for i, c := range cands {
		if !kindAgrees(q.Type, c.kind) || !anyTitle(c.titles, want) {
			continue
		}
		if found != nil {
			return nil // ambiguous
		}
		found = &cands[i]
	}
	return found
}

func anyTitle(titles []string, want map[string]bool) bool {
	for _, t := range titles {
		if want[titleKey(t)] {
			return true
		}
	}
	return false
}

func kindAgrees(series, cand domain.SeriesType) bool {
	return series == domain.TypeOther || cand == domain.TypeOther || series == cand
}

// straightQuotes replaces typographic quotes, which some search APIs do
// not match ("Academy’s" finds nothing on AniList, "Academy's" does).
var straightQuotes = strings.NewReplacer("’", "'", "‘", "'", "“", `"`, "”", `"`)

var trailingParens = regexp.MustCompile(`\s*\((novel|light novel|manga|manhwa|manhua|webtoon)\)\s*$`)

// titleKey normalizes a title for comparison: lowercase, without a
// trailing "(Novel)" and keeping only letters and digits (any script).
func titleKey(s string) string {
	s = trailingParens.ReplaceAllString(strings.ToLower(s), "")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// searchTitles are the titles to search, the series' own first.
func searchTitles(q Query) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range q.Titles {
		if k := titleKey(t); k != "" && !seen[k] && len(out) < maxSearches {
			seen[k] = true
			out = append(out, straightQuotes.Replace(strings.TrimSpace(t)))
		}
	}
	return out
}

var (
	tagRe    = regexp.MustCompile(`(?s)<[^>]*>|\[/?[a-zA-Z]+[^\]]*\]`)
	spacesRe = regexp.MustCompile(`[ \t]+`)
	blankRe  = regexp.MustCompile(`\n{3,}`)
)

// cleanSynopsis turns a description with HTML, BBCode or Markdown bold
// into plain text (the UI shows it as text, RULES §11).
func cleanSynopsis(s string) string {
	s = strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "**", "").Replace(s)
	s = html.UnescapeString(tagRe.ReplaceAllString(s, ""))
	s = blankRe.ReplaceAllString(spacesRe.ReplaceAllString(s, " "), "\n\n")
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxSynopsis {
		s = string(r[:maxSynopsis-1]) + "…"
	}
	return s
}
