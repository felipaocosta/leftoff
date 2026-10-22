package extract

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// chapterNumberRe finds the number after a chapter keyword in a label or URL:
// "Chapter 245", "Ch. 10.5", "Capítulo 3", "Episode 7", "chapter-12-5".
//
// A "-" is a decimal separator only before 1–2 digits (chapter-12-5 is
// 12.5); longer runs are ids, so ch1-310234 is chapter 1.
var chapterNumberRe = regexp.MustCompile(
	`(?i)\b(?:chapter|chap|ch|cap[ií]tulo|cap|episode|ep)[\s.\-_:#]*(\d+)(?:\.(\d+)|-(\d{1,2})(?:\D|$))?`)

// bareNumberRe matches a label that is only a number, e.g. "245".
var bareNumberRe = regexp.MustCompile(`^\s*(\d+(?:\.\d+)?)\s*$`)

// ParseChapterNumber extracts a chapter number from a label or URL, or
// returns nil when there is none (e.g. "Afterword").
func ParseChapterNumber(s string) *float64 {
	text := ""
	if m := chapterNumberRe.FindStringSubmatch(s); m != nil {
		text = m[1]
		if frac := m[2] + m[3]; frac != "" {
			text += "." + frac
		}
	} else if m := bareNumberRe.FindStringSubmatch(s); m != nil {
		text = m[1]
	}
	if text == "" {
		return nil
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil
	}
	return &n
}

// TextChunks returns the text of each run of adjacent text nodes under
// sel, in document order. Unlike Selection.Text, it keeps "Chapter 14" and
// "8 days ago" apart instead of gluing them into "Chapter 148 days ago",
// while text split by comments (React's "Chapter<!-- --> <!-- -->16") stays
// one chunk.
func TextChunks(sel *goquery.Selection) []string {
	var chunks []string
	var walk func(*goquery.Selection)
	walk = func(s *goquery.Selection) {
		var run strings.Builder
		flush := func() {
			if t := CleanText(run.String()); t != "" {
				chunks = append(chunks, t)
			}
			run.Reset()
		}
		s.Contents().Each(func(_ int, n *goquery.Selection) {
			switch goquery.NodeName(n) {
			case "#text":
				run.WriteString(n.Text())
			case "#comment":
			case "script", "style", "noscript", "svg":
				flush()
			default:
				flush()
				walk(n)
			}
		})
		flush()
	}
	walk(sel)
	return chunks
}

// CleanText trims s and collapses internal whitespace into single spaces.
func CleanText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
