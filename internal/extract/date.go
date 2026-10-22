package extract

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// absoluteLayouts are release date formats seen on scan sites.
var absoluteLayouts = []string{
	"January 2, 2006", "Jan 2, 2006", "January 2 2006", "Jan 2 2006",
	"2 January 2006", "2 Jan 2006", "2006-01-02", "2006/01/02",
	"02.01.2006", // day.month.year (Madara sites such as mangaread.org)
}

// abbrevRe matches an English date with an abbreviated month and an
// optional time after it, as Django sites print it: "Sept. 14, 2026, 2:18
// a.m.", "Oct. 3, 2026".
var abbrevRe = regexp.MustCompile(`^(?i)([a-z]{3,9})\.?\s+(\d{1,2}),?\s+(\d{4})(?:[,\s].*)?$`)

// relativeRe matches precise relative dates in en/pt/es: "8 days ago",
// "about 11 hours", "2d ago", "há 3 dias", "hace 2 semanas".
var relativeRe = regexp.MustCompile(`(?i)(\d+)\s*(seconds?|secs?|s|minutes?|mins?|m|hours?|hrs?|h|days?|d|weeks?|wks?|w|segundos?|minutos?|horas?|dias?|días?|semanas?)\b`)

// vagueRe matches units too coarse to date a release (months, years).
var vagueRe = regexp.MustCompile(`(?i)\d+\s*(months?|mos?|years?|yrs?|y|meses|m[eê]s|anos?|años?)\b`)

// ParseReleaseDate reads a chapter release date: an absolute date
// ("October 15, 2026") or a precise relative one ("8 days ago",
// "yesterday", "há 3 dias"), counted back from now in UTC. Months and years
// are too vague and are rejected, as is anything else (RULES §6.7). So is
// a date before 1990 or more than 2 days ahead (time zones): "99999 weeks
// ago" is a misreading, not a date.
func ParseReleaseDate(s string, now time.Time) (domain.Date, bool) {
	d, ok := parseReleaseDate(s, now)
	if !ok || d.String() < "1990-01-01" || d.String() > dateOf(now.UTC().AddDate(0, 0, 2)).String() || !d.Plausible() {
		return domain.Date{}, false
	}
	return d, true
}

func parseReleaseDate(s string, now time.Time) (domain.Date, bool) {
	s = CleanText(s)
	if s == "" || len(s) > 40 {
		return domain.Date{}, false
	}
	for _, layout := range absoluteLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return domain.NewDate(t.Year(), t.Month(), t.Day()), true
		}
	}
	if d, ok := parseAbbreviated(s); ok {
		return d, true
	}
	now = now.UTC()
	switch strings.ToLower(strings.Trim(s, " .")) {
	case "today", "hoje", "hoy", "just now", "agora", "ahora", "now":
		return dateOf(now), true
	case "yesterday", "ontem", "ayer":
		return dateOf(now.AddDate(0, 0, -1)), true
	}
	if vagueRe.MatchString(s) {
		return domain.Date{}, false
	}
	m := relativeRe.FindStringSubmatch(s)
	if m == nil {
		return domain.Date{}, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return domain.Date{}, false
	}
	unit := strings.ToLower(m[2])
	switch {
	case strings.HasPrefix(unit, "w"), strings.HasPrefix(unit, "sem"):
		return dateOf(now.AddDate(0, 0, -7*n)), true
	case strings.HasPrefix(unit, "d"):
		return dateOf(now.AddDate(0, 0, -n)), true
	case strings.HasPrefix(unit, "h"):
		return dateOf(now.Add(-time.Duration(n) * time.Hour)), true
	default: // minutes, seconds
		return dateOf(now), true
	}
}

func dateOf(t time.Time) domain.Date { return domain.NewDate(t.Year(), t.Month(), t.Day()) }

// parseAbbreviated reads abbrevRe dates; the month is recognized by its
// first three letters ("Sept." → September).
func parseAbbreviated(s string) (domain.Date, bool) {
	m := abbrevRe.FindStringSubmatch(s)
	if m == nil {
		return domain.Date{}, false
	}
	month, err := time.Parse("Jan", strings.ToUpper(m[1][:1])+strings.ToLower(m[1][1:3]))
	if err != nil {
		return domain.Date{}, false
	}
	day, _ := strconv.Atoi(m[2])  // \d{1,2}
	year, _ := strconv.Atoi(m[3]) // \d{4}
	t := time.Date(year, month.Month(), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day { // e.g. Feb. 31
		return domain.Date{}, false
	}
	return domain.NewDate(year, month.Month(), day), true
}
