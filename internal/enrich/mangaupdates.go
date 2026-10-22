package enrich

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
)

const mangaUpdatesAPI = "https://api.mangaupdates.com/v1"

// MangaUpdates finds series in MangaUpdates' public API (no key needed).
// It covers many manhwa and novels AniList does not have.
type MangaUpdates struct {
	f   fetch.Fetcher
	api string
}

// NewMangaUpdates returns the MangaUpdates finder; api "" is the official
// API base URL.
func NewMangaUpdates(f fetch.Fetcher, api string) *MangaUpdates {
	if api == "" {
		api = mangaUpdatesAPI
	}
	return &MangaUpdates{f: f, api: strings.TrimRight(api, "/")}
}

// Name implements Finder.
func (*MangaUpdates) Name() string { return domain.ExternalMangaUpdates }

type muGenre struct {
	Genre string `json:"genre"`
}

type muRecord struct {
	SeriesID int64     `json:"series_id"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Type     string    `json:"type"`
	Year     string    `json:"year"`
	Genres   []muGenre `json:"genres"`
}

// Find implements Finder: each search hit is matched by its title and the
// title the search matched (often an alternative title); the matching
// entry is then read in full.
func (m *MangaUpdates) Find(ctx context.Context, q Query) (*Match, error) {
	for _, title := range searchTitles(q) {
		hits, err := m.search(ctx, title)
		if err != nil {
			return nil, err
		}
		cands := make([]candidate, 0, len(hits))
		for _, h := range hits {
			cands = append(cands, candidate{
				titles: []string{h.Record.Title, h.HitTitle},
				kind:   muKind(h.Record.Type),
				load:   func(ctx context.Context) (*Match, error) { return m.series(ctx, h.Record.SeriesID) },
			})
		}
		if c := pick(q, cands); c != nil {
			return c.load(ctx)
		}
	}
	return nil, nil
}

type muHit struct {
	Record   muRecord `json:"record"`
	HitTitle string   `json:"hit_title"`
}

func (m *MangaUpdates) search(ctx context.Context, title string) ([]muHit, error) {
	body, err := json.Marshal(map[string]any{"search": title, "perpage": 10})
	if err != nil {
		return nil, fmt.Errorf("encode mangaupdates search: %w", err)
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	resp, err := m.f.Fetch(ctx, &fetch.Request{Method: http.MethodPost, URL: m.api + "/series/search", Header: h, Body: body})
	if err != nil {
		return nil, fmt.Errorf("mangaupdates search %q: %w", title, err)
	}
	var out struct {
		Results []muHit `json:"results"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("mangaupdates search %q: decode: %w", title, err)
	}
	return out.Results, nil
}

func (m *MangaUpdates) series(ctx context.Context, id int64) (*Match, error) {
	resp, err := fetch.Get(ctx, m.f, m.api+"/series/"+strconv.FormatInt(id, 10))
	if err != nil {
		return nil, fmt.Errorf("mangaupdates series %d: %w", id, err)
	}
	var s struct {
		muRecord
		Description string `json:"description"`
		// Status is about the original work, e.g. "128 Chapters (Ongoing)".
		// "completed" (not read) only says the scanlation is complete.
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp.Body, &s); err != nil {
		return nil, fmt.Errorf("mangaupdates series %d: decode: %w", id, err)
	}
	genres := make([]string, 0, len(s.Genres))
	for _, g := range s.Genres {
		genres = append(genres, g.Genre)
	}
	return &Match{
		Source: domain.ExternalMangaUpdates, ID: strconv.FormatInt(s.SeriesID, 10), URL: s.URL, Title: s.Title,
		Status: muStatus(s.Status), Genres: genres, Year: muYear(s.Year), Synopsis: cleanSynopsis(s.Description),
	}, nil
}

func muKind(t string) domain.SeriesType {
	switch strings.ToLower(t) {
	case "manga":
		return domain.TypeManga
	case "manhwa":
		return domain.TypeManhwa
	case "manhua":
		return domain.TypeManhua
	case "novel":
		return domain.TypeNovel
	}
	return domain.TypeOther
}

// muStatusRe finds the scanlation-independent status MangaUpdates puts in
// parentheses, e.g. "128 Chapters (Ongoing)".
var muStatusRe = regexp.MustCompile(`\(([A-Za-z /]+)\)\s*$`)

func muStatus(raw string) domain.Status {
	first, _, _ := strings.Cut(strings.TrimSpace(raw), "\n") // the line about the original work
	m := muStatusRe.FindStringSubmatch(first)
	if m == nil {
		return domain.StatusUnknown
	}
	text := strings.ToLower(m[1])
	if strings.Contains(text, "axed") || strings.Contains(text, "discontinued") {
		return domain.StatusCancelled // ended early ("Complete/Axed")
	}
	if part, _, _ := strings.Cut(text, "/"); part != "" {
		return domain.NormalizeStatus(part)
	}
	return domain.StatusUnknown
}

func muYear(s string) *int {
	if y, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && y > 0 {
		return &y
	}
	return nil
}
