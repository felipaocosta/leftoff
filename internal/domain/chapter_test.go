package domain

import (
	"strings"
	"testing"
)

func num(v float64) *float64 { return &v }

func TestChapterKey(t *testing.T) {
	tests := []struct {
		name   string
		number *float64
		label  string
		want   string
	}{
		{"integer number", num(245), "Chapter 245", "245"},
		{"fractional number", num(10.5), "Chapter 10.5", "10.5"},
		{"zero is a valid number", num(0), "Prologue", "0"},
		{"number ignores label", num(3), "anything", "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChapterKey(tt.number, tt.label); got != tt.want {
				t.Errorf("ChapterKey = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChapterKey_LabelBasedKeyIsStableAcrossFormatting(t *testing.T) {
	a := ChapterKey(nil, "Season 1 – Afterword (Part 3)")
	b := ChapterKey(nil, "  season 1 afterword   PART 3 ")
	if a != b {
		t.Errorf("keys differ: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, labelKeyPrefix) {
		t.Errorf("label key %q lacks prefix %q", a, labelKeyPrefix)
	}
	if c := ChapterKey(nil, "Extra"); c == a {
		t.Errorf("different labels produced the same key %q", c)
	}
}

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"  Season 1 – Afterword (Part 3) ", "season 1 afterword part 3"},
		{"Capítulo Extra!", "capítulo extra"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizeLabel(tt.in); got != tt.want {
			t.Errorf("NormalizeLabel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestChapterInfo_Key(t *testing.T) {
	c := ChapterInfo{Number: num(7), Label: "Ch. 7"}
	if c.Key() != "7" {
		t.Errorf("Key() = %q, want 7", c.Key())
	}
}
