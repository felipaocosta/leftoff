package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

const anilistEndpoint = "https://graphql.anilist.co"

// anilistQuery searches manga, manhwa, manhua and light novels (AniList
// keeps them all under type MANGA).
const anilistQuery = `query ($search: String) { Page(perPage: 10) { media(search: $search, type: MANGA) {
 id siteUrl format countryOfOrigin status startDate { year } genres synonyms description(asHtml: false)
 title { romaji english native } } } }`

// AniList finds series in AniList's public GraphQL API (no key needed).
type AniList struct {
	f        fetch.Fetcher
	endpoint string
}

// NewAniList returns the AniList finder; endpoint "" is the official API.
func NewAniList(f fetch.Fetcher, endpoint string) *AniList {
	if endpoint == "" {
		endpoint = anilistEndpoint
	}
	return &AniList{f: f, endpoint: endpoint}
}

// Name implements Finder.
func (*AniList) Name() string { return domain.ExternalAniList }

type anilistMedia struct {
	ID              int                 `json:"id"`
	SiteURL         string              `json:"siteUrl"`
	Format          string              `json:"format"`
	CountryOfOrigin string              `json:"countryOfOrigin"`
	Status          string              `json:"status"`
	StartDate       struct{ Year *int } `json:"startDate"`
	Genres          []string            `json:"genres"`
	Synonyms        []string            `json:"synonyms"`
	Description     string              `json:"description"`
	Title           struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
}

// Find implements Finder: it searches the series' titles in turn.
func (a *AniList) Find(ctx context.Context, q Query) (*Match, error) {
	for _, title := range searchTitles(q) {
		media, err := a.search(ctx, title)
		if err != nil {
			return nil, err
		}
		cands := make([]candidate, 0, len(media))
		for _, m := range media {
			cands = append(cands, candidate{
				titles: append([]string{m.Title.English, m.Title.Romaji, m.Title.Native}, m.Synonyms...),
				kind:   anilistKind(m),
				load:   func(context.Context) (*Match, error) { return anilistMatch(m), nil },
			})
		}
		if c := pick(q, cands); c != nil {
			return c.load(ctx)
		}
	}
	return nil, nil
}

func (a *AniList) search(ctx context.Context, title string) ([]anilistMedia, error) {
	body, err := json.Marshal(map[string]any{"query": anilistQuery, "variables": map[string]string{"search": title}})
	if err != nil {
		return nil, fmt.Errorf("encode anilist query: %w", err)
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")
	resp, err := a.f.Fetch(ctx, &fetch.Request{Method: http.MethodPost, URL: a.endpoint, Header: h, Body: body})
	if err != nil {
		return nil, fmt.Errorf("anilist search %q: %w", title, err)
	}
	var out struct {
		Data struct {
			Page struct {
				Media []anilistMedia `json:"media"`
			} `json:"Page"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("anilist search %q: decode: %w", title, err)
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("anilist search %q: %s", title, out.Errors[0].Message)
	}
	return out.Data.Page.Media, nil
}

// anilistKind is the series type of an entry: novels by format, comics by
// country of origin.
func anilistKind(m anilistMedia) domain.SeriesType {
	if m.Format == "NOVEL" {
		return domain.TypeNovel
	}
	switch m.CountryOfOrigin {
	case "JP":
		return domain.TypeManga
	case "KR":
		return domain.TypeManhwa
	case "CN", "TW":
		return domain.TypeManhua
	}
	return domain.TypeOther
}

var anilistStatus = map[string]domain.Status{
	"RELEASING": domain.StatusOngoing, "FINISHED": domain.StatusCompleted,
	"HIATUS": domain.StatusHiatus, "CANCELLED": domain.StatusCancelled,
}

func anilistMatch(m anilistMedia) *Match {
	title := m.Title.English
	if title == "" {
		title = m.Title.Romaji
	}
	status, ok := anilistStatus[m.Status]
	if !ok {
		status = domain.StatusUnknown
	}
	return &Match{
		Source: domain.ExternalAniList, ID: strconv.Itoa(m.ID), URL: m.SiteURL, Title: title,
		Status: status, Genres: m.Genres, Year: m.StartDate.Year, Synopsis: cleanSynopsis(m.Description),
	}
}
