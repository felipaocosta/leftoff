package mangadex

import (
	"maps"
	"slices"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// Subset of the MangaDex API responses used by the adapter.

type localized map[string]string

type mangaResponse struct {
	Data struct {
		ID            string          `json:"id"`
		Attributes    mangaAttributes `json:"attributes"`
		Relationships []relationship  `json:"relationships"`
	} `json:"data"`
}

type mangaAttributes struct {
	Title            localized   `json:"title"`
	AltTitles        []localized `json:"altTitles"`
	Description      localized   `json:"description"`
	Status           string      `json:"status"`
	OriginalLanguage string      `json:"originalLanguage"`
}

type relationship struct {
	Type       string `json:"type"`
	Attributes struct {
		FileName string `json:"fileName"`
	} `json:"attributes"`
}

type feedResponse struct {
	Data  []chapterData `json:"data"`
	Total int           `json:"total"`
}

type chapterData struct {
	ID         string `json:"id"`
	Attributes struct {
		Chapter            string `json:"chapter"`
		Title              string `json:"title"`
		TranslatedLanguage string `json:"translatedLanguage"`
		PublishAt          string `json:"publishAt"`
		ExternalURL        string `json:"externalUrl"`
	} `json:"attributes"`
}

// pick returns the first value present among the preferred languages, or
// any value when none matches.
func pick(m localized, prefer []string) string {
	for _, l := range prefer {
		if v := m[l]; v != "" {
			return v
		}
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		return m[k]
	}
	return ""
}

// altTitles lists the other titles (main title in other scripts and the
// alternative titles), without repeats.
func altTitles(attr mangaAttributes, main string) []string {
	seen := map[string]bool{main: true}
	out := []string{}
	add := func(m localized) {
		for _, k := range slices.Sorted(maps.Keys(m)) { // deterministic order
			if v := m[k]; v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	add(attr.Title)
	for _, m := range attr.AltTitles {
		add(m)
	}
	return out
}

func coverURL(id string, rels []relationship) string {
	for _, r := range rels {
		if r.Type == "cover_art" && r.Attributes.FileName != "" {
			return coverBase + "/" + id + "/" + r.Attributes.FileName + ".512.jpg"
		}
	}
	return ""
}

// typeFor maps the original language to the kind of work.
func typeFor(lang string) domain.SeriesType {
	switch lang {
	case "ja":
		return domain.TypeManga
	case "ko":
		return domain.TypeManhwa
	case "zh", "zh-hk":
		return domain.TypeManhua
	}
	return domain.TypeOther
}
