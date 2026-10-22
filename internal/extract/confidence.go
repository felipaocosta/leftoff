package extract

import "github.com/felipaocosta/leftoff/internal/domain"

// LowConfidence is the threshold below which the UI must ask the user to
// confirm the preview (SPEC §5.1).
const LowConfidence = 0.6

// Field weights; chapters weigh the most (SPEC §5.4).
const (
	weightTitle    = 0.30
	weightCover    = 0.10
	weightStatus   = 0.10
	weightChapters = 0.50
)

// fewChaptersMax caps the confidence of a guessed (fallback) result with
// fewer than fewChapters chapters: such a page usually shows only part of
// the list (the rest loads with JavaScript), so the preview must ask.
const fewChaptersMax = 0.5

// capFewChapters applies fewChaptersMax to a fallback result.
func capFewChapters(r *Result) {
	if len(r.Chapters) < fewChapters && r.Confidence > fewChaptersMax {
		r.Confidence = fewChaptersMax
	}
}

// Confidence scores a result from the fields it found. Chapters count fully
// when at least one has a number and half when none has.
func Confidence(info domain.SeriesInfo, chapters []domain.ChapterInfo) float64 {
	score := 0.0
	if info.Title != "" {
		score += weightTitle
	}
	if info.CoverURL != "" {
		score += weightCover
	}
	if info.Status != "" && info.Status != domain.StatusUnknown {
		score += weightStatus
	}
	if len(chapters) > 0 {
		score += weightChapters / 2
		for _, c := range chapters {
			if c.Number != nil {
				score += weightChapters / 2
				break
			}
		}
	}
	return score
}
