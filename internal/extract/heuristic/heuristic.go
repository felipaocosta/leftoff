// Package heuristic is the generic extractor used when no adapter matches a
// page (SPEC §5.4). It reads structured metadata (JSON-LD, Open Graph) and
// falls back to layout clues: the first <h1>, a "Status" label and the
// largest group of chapter-like links.
//
// It does NOT follow extra pages (paginated or AJAX chapter lists) nor
// guess the series type; low confidence results are confirmed by the user.
package heuristic

import (
	"context"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

// Method is the Result.Method of this extractor.
const Method = "heuristic"

// Extractor implements extract.Fallback.
type Extractor struct{ clock domain.Clock }

// New returns the heuristic extractor. The clock turns relative release
// dates ("8 days ago") into dates; it defaults to the system clock.
func New(clock ...domain.Clock) *Extractor {
	if len(clock) > 0 && clock[0] != nil {
		return &Extractor{clock: clock[0]}
	}
	return &Extractor{clock: domain.SystemClock{}}
}

// Extract reads everything from the already downloaded page; it never
// fetches. Missing fields are left empty and lower the confidence.
func (e *Extractor) Extract(_ context.Context, _ fetch.Fetcher, p *extract.Page) (*extract.Result, error) {
	ld := findJSONLD(p.Doc)
	info := domain.SeriesInfo{
		URL:      p.URL.String(),
		Title:    findTitle(p.Doc, ld, p.URL),
		CoverURL: findCover(p.Doc, ld, p.URL),
		Synopsis: findSynopsis(p.Doc, ld),
	}
	info.StatusRaw = findStatusText(p.Doc)
	if info.StatusRaw == "" || domain.NormalizeStatus(info.StatusRaw) == domain.StatusUnknown {
		if ld.Status != "" {
			info.StatusRaw = ld.Status
		}
	}
	info.Status = domain.NormalizeStatus(info.StatusRaw)
	info.Type = guessType(ld.Genre)
	if info.Type == domain.TypeOther {
		info.Type = guessType(findBadge(p.Doc, isTypeBadge))
	}
	chapters := findChapters(p.Doc, p.URL, e.clock.Now())
	if len(chapters) == 0 && isSiteName(info.Title, p.URL) {
		// Only the site's name and no chapters: a JavaScript app before it
		// renders, not a series (RULES §6.7). A single-series site whose
		// name is the series' has its chapter list, so it keeps the title.
		info.Title = ""
	}
	return &extract.Result{Series: info, Chapters: chapters, Method: Method}, nil
}

// guessType maps a JSON-LD genre such as "Manhwa" to a series type; the
// user can still correct it in the preview.
func guessType(genre string) domain.SeriesType {
	g := strings.ToLower(genre)
	for _, t := range []domain.SeriesType{domain.TypeManhwa, domain.TypeManhua, domain.TypeNovel, domain.TypeManga} {
		if strings.Contains(g, string(t)) {
			return t
		}
	}
	return domain.TypeOther
}

// isTypeBadge reports whether text is exactly a type name ("Manga",
// "Light Novel"), as shown in badges.
func isTypeBadge(text string) bool {
	switch strings.ToLower(text) {
	case "manga", "manhwa", "manhua", "novel", "light novel", "web novel":
		return true
	}
	return false
}
