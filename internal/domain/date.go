package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date is a calendar date without time of day (e.g. a chapter release date).
// The zero value means "unknown" and is serialized as JSON null.
type Date struct{ t time.Time }

// NewDate builds a Date from its components.
func NewDate(year int, month time.Month, day int) Date {
	return Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate parses a "YYYY-MM-DD" string.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("parse date %q: %w", s, ErrInvalidInput)
	}
	return Date{t: t}, nil
}

// IsZero reports whether the date is unknown.
func (d Date) IsZero() bool { return d.t.IsZero() }

// String returns "YYYY-MM-DD", or "" for the zero Date.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.t.Format(dateLayout)
}

// Plausible reports whether the date can be a release date: from 1990 to
// the year 9999 (the last one "YYYY-MM-DD" can write).
func (d Date) Plausible() bool {
	return !d.IsZero() && d.t.Year() >= 1990 && d.t.Year() <= 9999
}

// MarshalJSON encodes the date as "YYYY-MM-DD" or null. A year outside
// 0–9999 is refused: it could not be read back, and the library would not
// open again.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	if y := d.t.Year(); y < 0 || y > 9999 {
		return nil, fmt.Errorf("encode date: year %d: %w", y, ErrInvalidInput)
	}
	return json.Marshal(d.String())
}

// UnmarshalJSON accepts "YYYY-MM-DD", "" or null.
func (d *Date) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		*d = Date{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("decode date: %w", err)
	}
	if s == "" {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
