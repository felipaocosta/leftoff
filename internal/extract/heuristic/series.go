package heuristic

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
)

var (
	// titleSeparatorRe splits "Title - Site Name" / "Title | Site".
	titleSeparatorRe = regexp.MustCompile(`\s+[-|–—]\s+`)
	readPrefixRe     = regexp.MustCompile(`(?i)^read\s+`)
	// typeSuffixRe is the series type that sites append to decorated
	// titles ("Solo Leveling Manhwa | Read at Site").
	// decorationRe marks the site part of a decorated title.
	decorationRe   = regexp.MustCompile(`(?i)\b(read|online|free|scans?)\b`)
	typeSuffixRe   = regexp.MustCompile(`(?i)\s+(manga|manhwa|manhua|webtoon|comic)$`)
	onlineSuffixRe = regexp.MustCompile(`(?i)\s+(?:(?:manga|manhwa|manhua|comic)\s+)?online(?:\s+free)?$`)
	// statusLabelRe matches a "Status" label in pt/en/es.
	statusLabelRe = regexp.MustCompile(`(?i)^\s*(status|estado|situa[cç][aã]o)\s*:?\s*`)
	// skipImageRe filters images that are never a cover.
	skipImageRe = regexp.MustCompile(`(?i)(logo|avatar|icon|banner|loading|placeholder)|\.svg(\?|$)|^data:`)
)

// findTitle follows SPEC §5.4: JSON-LD name → og:title (cleaned) → <h1>.
// Sites often decorate the structured title ("X Manhwa | Read Online Free
// at site.com") or put only their own name in og:title ("MangaFire"), so
// when a candidate contains the <h1>, or is just the site name, the <h1>
// wins.
func findTitle(doc *goquery.Document, ld ldWork, base *url.URL) string {
	h1 := extract.CleanText(doc.Find("h1").First().Text())
	candidates := []string{cleanLDName(ld.Name, base), cleanOGTitle(metaContent(doc, "og:title"), base)}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if h1 != "" && (strings.Contains(strings.ToLower(c), strings.ToLower(h1)) || isSiteName(c, base)) {
			return h1
		}
		return c
	}
	return h1
}

// isSiteName reports whether title is just the site's name, e.g.
// "MangaFire" on mangafire.to.
func isSiteName(title string, base *url.URL) bool {
	letters := strings.ToLower(strings.Join(strings.FieldsFunc(title, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), ""))
	return letters != "" && strings.Contains(strings.ToLower(base.Hostname()), letters)
}

// cleanOGTitle turns "Read Solo Leveling Manga Online - Site" and
// "Solo Leveling Manhwa | Read Online Free at Site" into "Solo Leveling".
func cleanOGTitle(s string, base *url.URL) string {
	s = extract.CleanText(s)
	parts := titleSeparatorRe.Split(s, 2)
	s = parts[0]
	s = readPrefixRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(onlineSuffixRe.ReplaceAllString(s, ""))
	if len(parts) > 1 && isDecoration(parts[1], base) {
		s = strings.TrimSpace(typeSuffixRe.ReplaceAllString(s, ""))
	}
	return s
}

// isDecoration reports whether the part after a title separator is the
// site's ("Read Online Free at site.org", "Site Scans") and not a subtitle.
func isDecoration(s string, base *url.URL) bool {
	return decorationRe.MatchString(s) || isSiteName(s, base)
}

// cleanLDName drops the site decoration some sites put in the JSON-LD name
// ("Solo Leveling Manhwa | Read Online Free at site.org"). Unlike og:title,
// the name is only cut when the part after the separator reads as
// decoration, so "Re:Zero - Starting Life in Another World" stays whole.
func cleanLDName(s string, base *url.URL) string {
	s = extract.CleanText(s)
	parts := titleSeparatorRe.Split(s, 2)
	if len(parts) < 2 || !isDecoration(parts[1], base) {
		return s
	}
	return strings.TrimSpace(typeSuffixRe.ReplaceAllString(parts[0], ""))
}

// findCover: JSON-LD image → an <img> marked as the cover (itemprop=image,
// alt "Cover…"/"Capa…") → og:image, unless it is a generated social banner
// → the largest <img> near the <h1>.
func findCover(doc *goquery.Document, ld ldWork, base *url.URL) string {
	if ld.Image != "" {
		return ld.Image
	}
	if c := markedCover(doc); c != "" {
		return extract.ResolveURL(base, c)
	}
	if og := metaContent(doc, "og:image"); og != "" && !isSocialBanner(doc, og) {
		return og
	}
	if named := namedCover(doc); named != "" {
		return extract.ResolveURL(base, named)
	}
	if near := imageNearTitle(doc, base); near != "" {
		return near
	}
	return metaContent(doc, "og:image") // a banner is still better than nothing
}

// coverAltRe matches alt texts that name the image as the cover.
var coverAltRe = regexp.MustCompile(`(?i)^\s*(cover|capa|portada|poster)\b`)

// markedCover returns the src of an <img> the page itself marks as the
// cover, skipping lazy-loading placeholders.
func markedCover(doc *goquery.Document) string {
	var src string
	doc.Find("img").EachWithBreak(func(_ int, img *goquery.Selection) bool {
		if img.AttrOr("itemprop", "") != "image" && !coverAltRe.MatchString(img.AttrOr("alt", "")) {
			return true
		}
		if s := imageSrc(img); s != "" && !skipImageRe.MatchString(s) {
			src = s
			return false
		}
		return true
	})
	return src
}

// coverFileRe matches image file names that say they are the cover
// ("Eternally-Regressing-Knight-Cover.webp").
var coverFileRe = regexp.MustCompile(`(?i)(^|[-_.\s])(cover|capa|portada)([-_.\s]|$)`)

// namedCover returns the first image whose file name says it is the cover.
// It runs after og:image, since pages listing other series also have
// covers named like that.
func namedCover(doc *goquery.Document) string {
	var src string
	doc.Find("img").EachWithBreak(func(_ int, img *goquery.Selection) bool {
		s := imageSrc(img)
		if s == "" || skipImageRe.MatchString(s) {
			return true
		}
		if u, err := url.Parse(s); err == nil && coverFileRe.MatchString(path.Base(u.Path)) {
			src = s
			return false
		}
		return true
	})
	return src
}

// socialBannerRe matches og:image URLs generated for link previews.
var socialBannerRe = regexp.MustCompile(`(?i)og[-_]?image|opengraph|social[-_]?(card|image)|share[-_]?image`)

// isSocialBanner reports whether og:image is a generated link-preview
// banner (by URL, or landscape by its declared size) rather than a cover.
func isSocialBanner(doc *goquery.Document, og string) bool {
	if socialBannerRe.MatchString(og) {
		return true
	}
	w, _ := strconv.Atoi(metaContent(doc, "og:image:width"))
	h, _ := strconv.Atoi(metaContent(doc, "og:image:height"))
	return w > 0 && h > 0 && w > h
}

// imageNearTitle looks for images in the closest ancestors of the <h1>
// (up to 4 levels) and returns the one with the largest declared area, or
// the first one when none declares a size.
func imageNearTitle(doc *goquery.Document, base *url.URL) string {
	scope := doc.Find("h1").First()
	for range 4 {
		scope = scope.Parent()
		if scope.Length() == 0 {
			break
		}
		best, bestArea := "", -1
		scope.Find("img").Each(func(_ int, img *goquery.Selection) {
			src := imageSrc(img)
			if src == "" || skipImageRe.MatchString(src) {
				return
			}
			if area := attrInt(img, "width") * attrInt(img, "height"); area > bestArea {
				best, bestArea = src, area
			}
		})
		if best != "" {
			return extract.ResolveURL(base, best)
		}
	}
	return ""
}

// imageSrc prefers lazy-loading attributes, which hold the real URL.
func imageSrc(img *goquery.Selection) string {
	for _, attr := range []string{"data-src", "data-lazy-src", "src"} {
		if v := strings.TrimSpace(img.AttrOr(attr, "")); v != "" {
			return v
		}
	}
	return ""
}

// findSynopsis: JSON-LD description → og:description → meta description.
func findSynopsis(doc *goquery.Document, ld ldWork) string {
	for _, s := range []string{ld.Description, metaContent(doc, "og:description"), metaContent(doc, "description")} {
		if s = extract.CleanText(s); s != "" {
			return s
		}
	}
	return ""
}

// findStatusText returns the text next to a "Status:" label, or "".
// It looks, in order, inside the label element itself
// ("<div>Status <i>Ongoing</i></div>"), at its next sibling
// ("<dt>Status</dt><dd>Ongoing</dd>") and at its parent
// ("<li><b>Status:</b> Ongoing</li>").
func findStatusText(doc *goquery.Document) string {
	if v := findLabelledStatus(doc); v != "" {
		return v
	}
	return findBadge(doc, func(text string) bool { return domain.NormalizeStatus(text) != domain.StatusUnknown })
}

// findBadge returns the first short element near the <h1> (up to 4
// ancestors) whose whole text satisfies ok, e.g. a "Releasing" status badge
// or a "Manga" type badge. Searching only near the title avoids menus.
func findBadge(doc *goquery.Document, ok func(string) bool) string {
	scope := doc.Find("h1").First()
	for range 4 {
		scope = scope.Parent()
		if scope.Length() == 0 {
			return ""
		}
		var found string
		scope.Find("span, div, a, li, td, p, small, b, strong").EachWithBreak(func(_ int, el *goquery.Selection) bool {
			// The element's own text must be all of its text: a parent glues
			// its badges together ("MangaReleasing2022"), while a badge with
			// an empty icon child ("<span><i></i>Releasing</span>") is fine.
			text := extract.CleanText(el.Text())
			if extract.CleanText(el.Contents().Not("*").Text()) != text {
				return true
			}
			if len(text) > 0 && len(text) <= 20 && ok(text) {
				found = text
				return false
			}
			return true
		})
		if found != "" {
			return found
		}
	}
	return ""
}

// findLabelledStatus returns the text next to a "Status:" label, or "".
func findLabelledStatus(doc *goquery.Document) string {
	var found string
	doc.Find("body *").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		own := extract.CleanText(s.Contents().Not("*").Text())
		if !statusLabelRe.MatchString(own) || len(own) > 40 {
			return true
		}
		for _, candidate := range []string{s.Text(), s.Next().Text(), s.Parent().Text()} {
			v := extract.CleanText(statusLabelRe.ReplaceAllString(extract.CleanText(candidate), ""))
			if v != "" && len(v) <= 40 {
				found = v
				return false
			}
		}
		return true
	})
	return found
}

func metaContent(doc *goquery.Document, name string) string {
	sel := doc.Find(`meta[property="` + name + `"], meta[name="` + name + `"]`).First()
	return strings.TrimSpace(sel.AttrOr("content", ""))
}

func attrInt(s *goquery.Selection, attr string) int {
	n, err := strconv.Atoi(strings.TrimSuffix(s.AttrOr(attr, ""), "px"))
	if err != nil {
		return 0
	}
	return n
}
