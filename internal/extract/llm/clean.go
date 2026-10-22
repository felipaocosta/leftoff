package llm

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Limits of the cleaned page sent to the model (SPEC §5.5).
const (
	maxTextRunes  = 160 // longer text nodes (synopses) are cut
	maxAttrRunes  = 120 // longer attribute values are cut
	maxClasses    = 4   // class names kept per element
	keepRepeated  = 4   // runs of similar siblings keep this many
	repeatedLimit = 6   // runs longer than this are collapsed
)

// dropped elements carry no data a recipe can select.
var dropped = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Svg: true, atom.Noscript: true,
	atom.Iframe: true, atom.Link: true, atom.Template: true, atom.Canvas: true,
	atom.Video: true, atom.Audio: true, atom.Source: true,
	atom.Input: true, atom.Button: true, atom.Select: true, atom.Textarea: true,
	atom.Nav: true, atom.Footer: true,
}

// keptAttrs are the attributes selectors and values usually rely on.
var keptAttrs = map[string]bool{
	"id": true, "class": true, "href": true, "src": true, "data-src": true,
	"alt": true, "title": true, "itemprop": true, "property": true,
	"content": true, "datetime": true, "rel": true, "name": true,
}

// CleanHTML returns a compact copy of the page for the prompt: no scripts,
// styles or SVG, only useful attributes, short texts, long runs of similar
// siblings (chapter lists) reduced to a few examples, and at most maxBytes
// bytes. The page itself is not changed.
func CleanHTML(doc *goquery.Document, maxBytes int) (string, error) {
	var raw bytes.Buffer
	for _, n := range doc.Nodes {
		if err := html.Render(&raw, n); err != nil {
			return "", fmt.Errorf("render page: %w", err)
		}
	}
	root, err := html.Parse(&raw) // a private copy to prune
	if err != nil {
		return "", fmt.Errorf("reparse page: %w", err)
	}
	prune(root)
	var out bytes.Buffer
	if err := html.Render(&out, root); err != nil {
		return "", fmt.Errorf("render cleaned page: %w", err)
	}
	s := out.String()
	if len(s) > maxBytes {
		s = strings.ToValidUTF8(s[:maxBytes], "") + "\n<!-- truncated -->"
	}
	return s, nil
}

// prune cleans n's subtree in place.
func prune(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if removable(c) {
			n.RemoveChild(c)
		} else {
			cleanNode(c)
			prune(c)
			if c.Type == html.ElementNode && c.FirstChild == nil && !keepEmpty(c) {
				n.RemoveChild(c)
			}
		}
		c = next
	}
	collapseRuns(n)
}

func removable(n *html.Node) bool {
	switch n.Type {
	case html.CommentNode, html.DoctypeNode:
		return true
	case html.TextNode:
		return strings.TrimSpace(n.Data) == ""
	case html.ElementNode:
		if dropped[n.DataAtom] {
			return true
		}
		// In <head>, only the title and og:/description metadata matter.
		return n.DataAtom == atom.Meta && !strings.HasPrefix(attr(n, "property"), "og:") && attr(n, "name") != "description"
	}
	return false
}

// keepEmpty keeps childless elements that still hold data in attributes.
func keepEmpty(n *html.Node) bool {
	switch n.DataAtom {
	case atom.Img, atom.Meta, atom.Time:
		return true
	case atom.A:
		return attr(n, "href") != ""
	}
	return false
}

func cleanNode(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		n.Data = cut(strings.Join(strings.Fields(n.Data), " "), maxTextRunes)
	case html.ElementNode:
		kept := n.Attr[:0]
		for _, a := range n.Attr {
			if !keptAttrs[a.Key] || strings.HasPrefix(a.Val, "data:") {
				continue
			}
			if a.Key == "class" {
				if a.Val = simpleClasses(a.Val); a.Val == "" {
					continue
				}
			}
			a.Val = cut(a.Val, maxAttrRunes)
			kept = append(kept, a)
		}
		n.Attr = kept
	}
}

// collapseRuns keeps the first few of every run of similar element
// siblings (same tag and class) and notes how many were left out.
func collapseRuns(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		sig, run := signature(c), []*html.Node{c}
		for d := c.NextSibling; d != nil; d = d.NextSibling {
			if d.Type == html.ElementNode {
				if signature(d) != sig {
					break
				}
				run = append(run, d)
			}
		}
		if len(run) <= repeatedLimit {
			c = run[len(run)-1]
			continue
		}
		note := &html.Node{Type: html.CommentNode, Data: fmt.Sprintf(" %d more similar <%s> elements ", len(run)-keepRepeated, c.Data)}
		n.InsertBefore(note, run[keepRepeated])
		for _, d := range run[keepRepeated:] {
			n.RemoveChild(d)
		}
		c = note
	}
}

// simpleClasses keeps the first few class names usable in a selector,
// leaving out utility classes such as "md:flex" or "w-[140px]".
func simpleClasses(v string) string {
	var out []string
	for _, c := range strings.Fields(v) {
		if len(out) == maxClasses {
			break
		}
		if len(c) <= 40 && !strings.ContainsAny(c, `:[]/()!&.#%@`) {
			out = append(out, c)
		}
	}
	return strings.Join(out, " ")
}

// signature groups similar siblings; <meta> tags differ by property.
func signature(n *html.Node) string {
	return n.Data + "." + attr(n, "class") + "." + attr(n, "property")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func cut(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
