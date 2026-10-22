package extract

import (
	"testing"
	"time"
)

func TestParseReleaseDate(t *testing.T) {
	now := time.Date(2026, 10, 20, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		in   string
		want string // "" means rejected
	}{
		{"October 15, 2026", "2026-10-15"},
		{"Oct 9, 2026", "2026-10-09"},
		{"Sept. 14, 2026, 2:18 a.m.", "2026-09-14"}, // Django (mgeko)
		{"Oct. 3, 2026", "2026-10-03"},
		{"13.10.2026", "2026-10-13"}, // Madara (mangaread)
		{"31.02.2026", ""},
		{"June 1, 2026, 10 p.m.", "2026-06-01"},
		{"Feb. 31, 2026", ""},
		{"Foo. 3, 2026", ""},
		{"2026-09-24", "2026-09-24"},
		{"8 days ago", "2026-10-12"},
		{"8 days", "2026-10-12"},
		{"2d ago", "2026-10-18"},
		{"about 11 hours", "2026-10-19"},
		{"5 hours ago", "2026-10-19"},
		{"2 hours ago", "2026-10-20"},
		{"30 minutes ago", "2026-10-20"},
		{"3 weeks ago", "2026-09-29"},
		{"yesterday", "2026-10-19"},
		{"Hoje", "2026-10-20"},
		{"há 3 dias", "2026-10-17"},
		{"hace 2 días", "2026-10-18"},
		{"há 2 semanas", "2026-10-06"},
		{"about 1 month", ""},
		{"5mo ago", ""},
		{"2 years ago", ""},
		{"há 2 meses", ""},
		{"Chapter 14", ""},
		{"New", ""},
		{"", ""},
	}
	for _, tt := range tests {
		d, ok := ParseReleaseDate(tt.in, now)
		if got := d.String(); got != tt.want || ok != (tt.want != "") {
			t.Errorf("ParseReleaseDate(%q) = %q, %v; want %q", tt.in, got, ok, tt.want)
		}
	}
}
