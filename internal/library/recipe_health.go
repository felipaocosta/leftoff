package library

import (
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract/recipe"
)

// brokenAfter is how many failed checks in a row mark a recipe as broken.
const brokenAfter = 3

// Reasons a series says its site's recipe stopped working (RF-55).
const (
	ReasonFallback = "fallback" // read by the heuristic instead of the recipe
	ReasonFailing  = "failing"  // the recipe keeps failing
)

// SeriesLister lists the library (implemented by *Service).
type SeriesLister interface {
	List() []domain.Series
}

// FailingSeries is a series whose site recipe did not work on its last
// checks, with the page to regenerate the recipe from.
type FailingSeries struct {
	SeriesID string `json:"series_id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Reason   string `json:"reason"`
	Message  string `json:"message,omitempty"`
}

// WithSeries lets List report the series whose recipe stopped working.
func (r *Recipes) WithSeries(l SeriesLister) *Recipes {
	r.series = l
	return r
}

// failing returns the series of rc's sites checked after rc was last
// changed whose check fell back to the heuristic or failed brokenAfter
// times in a row. The recipe is never regenerated here: the user does it
// from the Recipes screen (SPEC §5.5).
func (r *Recipes) failing(rc *recipe.Recipe) []FailingSeries {
	out := []FailingSeries{}
	if r.series == nil {
		return out
	}
	sites := map[string]bool{rc.Domain: true}
	for _, a := range rc.Aliases {
		sites[a] = true
	}
	for _, sr := range r.series.List() {
		if !sites[strings.TrimPrefix(strings.ToLower(sr.Source.Domain), "www.")] || !checkedAfter(sr, rc.UpdatedAt) {
			continue
		}
		f := FailingSeries{SeriesID: sr.ID, Title: sr.Title, URL: sr.Source.URL}
		switch {
		case sr.CheckError != nil && sr.CheckError.Failures >= brokenAfter && sr.Source.Method == rc.Method():
			f.Reason, f.Message = ReasonFailing, sr.CheckError.Message
		case strings.HasPrefix(sr.Source.Method, "heuristic"):
			f.Reason = ReasonFallback
		default:
			continue
		}
		out = append(out, f)
	}
	return out
}

// checkedAfter reports whether the series' last check (or add) happened
// after t, so its result reflects the current recipe.
func checkedAfter(sr domain.Series, t time.Time) bool {
	last := sr.AddedAt
	if sr.LastCheckedAt != nil && sr.LastCheckedAt.After(last) {
		last = *sr.LastCheckedAt
	}
	if sr.CheckError != nil && sr.CheckError.At.After(last) {
		last = sr.CheckError.At
	}
	return last.After(t)
}
