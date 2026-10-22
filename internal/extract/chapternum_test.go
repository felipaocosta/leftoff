package extract

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestParseChapterNumber(t *testing.T) {
	tests := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"Chapter 245", 245, true},
		{"Ch. 10.5", 10.5, true},
		{"Capítulo 3", 3, true},
		{"Cap 12", 12, true},
		{"Episode 7 - The Return", 7, true},
		{"Vol.2 Ch.15", 15, true},
		{"https://a.org/manga/x/chapter-12-5/", 12.5, true},
		{"https://a.org/manga/x/chapter-245/", 245, true},
		{"245", 245, true},
		{"Afterword", 0, false},
		{"Bunch 5", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		got := ParseChapterNumber(tt.in)
		switch {
		case !tt.ok && got != nil:
			t.Errorf("%q: got %v, want nil", tt.in, *got)
		case tt.ok && (got == nil || *got != tt.want):
			t.Errorf("%q: got %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestCleanText(t *testing.T) {
	if got := CleanText("  Chapter\n\t 245  "); got != "Chapter 245" {
		t.Errorf("CleanText = %q", got)
	}
}

func TestParseChapterNumber_DashSeparatedIDsAreNotDecimals(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"https://roliascan.com/read/x/ch1-310234/", 1},
		{"https://a.org/x/chapter-12-5/", 12.5},
		{"https://a.org/x/chapter-12-50", 12.5},
		{"https://a.org/x/chapter-7-the-end", 7},
		{"Chapter 3.25", 3.25},
	}
	for _, tt := range tests {
		if got := ParseChapterNumber(tt.in); got == nil || *got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestTextChunks_KeepsTextNodesApartInOrder(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<a><div><img alt="x"><span>Chapter<!-- --> <!-- -->14</span></div><span>8 days ago</span><script>var x</script> New </a>`))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(TextChunks(doc.Find("a")), "|")
	if got != "Chapter 14|8 days ago|New" {
		t.Errorf("TextChunks = %q", got)
	}
}
