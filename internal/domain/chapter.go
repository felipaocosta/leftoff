package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// labelKeyPrefix marks keys derived from a label instead of a number, so they
// can never collide with a numeric key like "245".
const labelKeyPrefix = "l:"

// Chapter is a chapter stored in the library (SPEC §6.2).
//
// Read state is tied to Key, never to URL, so it survives URL and domain
// changes on the source (RF-14).
type Chapter struct {
	Key        string   `json:"key"`
	Number     *float64 `json:"number"`
	Label      string   `json:"label"`
	URL        string   `json:"url"`
	AltURLs    []string `json:"alt_urls"`
	ReleasedAt Date     `json:"released_at"`
	// DiscoveredAt is when a refresh first found this chapter; it is nil for
	// chapters imported when the series was added. It stands in for
	// ReleasedAt in the release estimate when the source has no dates.
	DiscoveredAt *time.Time `json:"discovered_at"`
	Read         bool       `json:"read"`
	ReadAt       *time.Time `json:"read_at"`
	Extra        Extra      `json:"-"`
}

// ChapterKey returns the stable key of a chapter (SPEC §5.6): the number when
// present ("245", "10.5"), otherwise a hash of the normalized label.
func ChapterKey(number *float64, label string) string {
	if number != nil {
		return strconv.FormatFloat(*number, 'f', -1, 64)
	}
	sum := sha256.Sum256([]byte(NormalizeLabel(label)))
	return labelKeyPrefix + hex.EncodeToString(sum[:6])
}

// NormalizeLabel lowercases a label, turns punctuation into spaces and
// collapses whitespace: "  Season 1 – Afterword (Part 3) " becomes
// "season 1 afterword part 3".
func NormalizeLabel(label string) string {
	mapped := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, label)
	return strings.Join(strings.Fields(mapped), " ")
}

// ReleaseDate returns the date used for release estimates: the source's
// release date when known, otherwise the date the chapter was discovered,
// otherwise the zero Date.
func (c Chapter) ReleaseDate() Date {
	if !c.ReleasedAt.IsZero() || c.DiscoveredAt == nil {
		return c.ReleasedAt
	}
	t := c.DiscoveredAt.UTC()
	return NewDate(t.Year(), t.Month(), t.Day())
}
