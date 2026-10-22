package heuristic

import (
	"encoding/json"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// workTypes are schema.org types that describe the series itself.
var workTypes = map[string]bool{
	"book": true, "comicseries": true, "creativeworkseries": true,
	"creativework": true, "bookseries": true, "periodical": true,
}

// ldWork holds the JSON-LD fields we use from a work node.
type ldWork struct {
	Name        string
	Image       string
	Description string
	Status      string // non-standard, but common on manga sites
	Genre       string // e.g. "Manhwa"; used to guess the series type
}

// findJSONLD returns the first schema.org work node on the page, or the
// zero value. Malformed scripts are skipped.
func findJSONLD(doc *goquery.Document) ldWork {
	var found ldWork
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		var v any
		if json.Unmarshal([]byte(s.Text()), &v) != nil {
			return true // ignore broken JSON-LD, try the next script
		}
		if node := findWorkNode(v); node != nil {
			found = ldWork{
				Name:        asString(node["name"]),
				Image:       asImage(node["image"]),
				Description: asString(node["description"]),
				Status:      asString(node["status"]),
				Genre:       asGenre(node["genre"]),
			}
			return false
		}
		return true
	})
	return found
}

// findWorkNode walks objects, arrays and @graph looking for a work type.
func findWorkNode(v any) map[string]any {
	switch t := v.(type) {
	case []any:
		for _, item := range t {
			if n := findWorkNode(item); n != nil {
				return n
			}
		}
	case map[string]any:
		if isWorkType(t["@type"]) {
			return t
		}
		if g, ok := t["@graph"]; ok {
			return findWorkNode(g)
		}
	}
	return nil
}

func isWorkType(v any) bool {
	switch t := v.(type) {
	case string:
		return workTypes[strings.ToLower(t)]
	case []any:
		for _, item := range t {
			if isWorkType(item) {
				return true
			}
		}
	}
	return false
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// asImage accepts "url", {"url": "..."} or a list of either.
func asImage(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]any:
		return asString(t["url"])
	case []any:
		if len(t) > 0 {
			return asImage(t[0])
		}
	}
	return ""
}

// asGenre accepts "Manhwa" or ["Action", "Manhwa"] and joins lists.
func asGenre(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}
