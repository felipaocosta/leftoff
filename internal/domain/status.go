package domain

import (
	"strings"
	"unicode"
)

// Status is the normalized publication status of a series (SPEC §5.7).
type Status string

// Known statuses.
const (
	StatusOngoing   Status = "ongoing"
	StatusCompleted Status = "completed"
	StatusHiatus    Status = "hiatus"
	StatusCancelled Status = "cancelled"
	StatusUnknown   Status = "unknown"
)

// statusKeywords maps keywords (lowercase, letters only, no accents) to a
// status. Order matters: the first group with a match wins, so more specific
// statuses come before generic ones.
var statusKeywords = []struct {
	status   Status
	keywords []string
}{
	{StatusHiatus, []string{"hiatus", "hiato", "onhold", "pausado", "pausa"}},
	{StatusCancelled, []string{"cancelled", "canceled", "cancelado", "dropped", "discontinued", "abandonado"}},
	{StatusCompleted, []string{"completed", "complete", "completo", "concluido", "finalizado", "finished", "terminado"}},
	{StatusOngoing, []string{"ongoing", "emandamento", "enandamento", "encurso", "emlancamento", "publishing", "releasing", "updating"}},
}

// NormalizeStatus maps a raw status text in pt/en/es (e.g. "OnGoing",
// "Em andamento", "Finalizado") to a Status. Unrecognized text returns
// StatusUnknown.
func NormalizeStatus(raw string) Status {
	folded := foldLetters(raw)
	if folded == "" {
		return StatusUnknown
	}
	for _, group := range statusKeywords {
		for _, kw := range group.keywords {
			if strings.Contains(folded, kw) {
				return group.status
			}
		}
	}
	return StatusUnknown
}

// accentFold covers the accented letters used in pt/es status texts.
var accentFold = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a",
	"é", "e", "ê", "e", "í", "i", "ó", "o", "ô", "o", "õ", "o",
	"ú", "u", "ü", "u", "ç", "c", "ñ", "n",
)

// foldLetters lowercases s, removes accents and drops every non-letter, so
// "On-Going" and "Em Andamento" become "ongoing" and "emandamento".
func foldLetters(s string) string {
	s = accentFold.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
