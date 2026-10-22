package extract

import (
	"math"
	"strings"
	"testing"
	"time"
)

func FuzzParseChapterNumber(f *testing.F) {
	for _, s := range []string{"Chapter 245", "Ch. 10.5", "chapter-12-5", "ch1-310234", "245", "Capítulo 3", "Episode 7",
		"Chapter 99999999999999999999999", "Afterword", "chapter-1e9", "ch.-.5"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		n := ParseChapterNumber(s)
		if n == nil {
			return
		}
		if math.IsNaN(*n) || math.IsInf(*n, 0) || *n < 0 {
			t.Fatalf("ParseChapterNumber(%q) = %v", s, *n)
		}
	})
}

func FuzzParseReleaseDate(f *testing.F) {
	for _, s := range []string{"October 15, 2026", "8 days ago", "há 3 dias", "hace 2 semanas", "yesterday", "Sept. 14, 2026, 2:18 a.m.",
		"14.09.2026", "2 hours ago", "99999999999 hours ago", "9999999999 weeks ago", "1 month ago"} {
		f.Add(s)
	}
	now := time.Date(2026, 10, 20, 21, 0, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, s string) {
		d, ok := ParseReleaseDate(s, now)
		if !ok {
			return
		}
		got, err := time.Parse(time.DateOnly, d.String())
		if err != nil {
			t.Fatalf("ParseReleaseDate(%q) = %q, not a date", s, d)
		}
		// A relative date is counted back from now: never in the future.
		if relativeRe.MatchString(CleanText(s)) && !looksAbsolute(s) && got.After(now) {
			t.Fatalf("ParseReleaseDate(%q) = %s, after now", s, d)
		}
	})
}

// looksAbsolute reports whether s has a 4-digit year, i.e. it may have
// been read as an absolute date.
func looksAbsolute(s string) bool {
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
		if len(f) == 4 {
			return true
		}
	}
	return false
}
