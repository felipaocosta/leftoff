//go:build demo

package app

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// TestWriteDemoLibrary writes a library of made-up series to DEMO_DATA_DIR,
// for screenshots and for trying the interface without adding anything:
//
//	DEMO_DATA_DIR=/tmp/leftoff-demo go test -tags demo -run TestWriteDemoLibrary ./internal/app
//	DATA_DIR=/tmp/leftoff-demo CHECK_INTERVAL_HOURS=0 go run ./cmd/server
//
// The titles, covers (abstract shapes) and sites (example.org) are invented.
func TestWriteDemoLibrary(t *testing.T) {
	dir := os.Getenv("DEMO_DATA_DIR")
	if dir == "" {
		t.Skip("DEMO_DATA_DIR not set")
	}
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	store, err := storage.Open(context.Background(), dir, domain.FixedClock{T: now}, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i, d := range demoSeries() {
		s := d.series(i, now)
		if err := store.Update(ctx, func(l *storage.Library) error {
			l.Series = append(l.Series, s)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		local, err := store.SaveCover(ctx, s.ID, demoCover(i))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update(ctx, func(l *storage.Library) error {
			j := l.SeriesByID(s.ID)
			l.Series[j].Cover.Local, l.Series[j].Cover.UpdatedAt = local, &now
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

type demo struct {
	title, site, synopsis string
	typ                   domain.SeriesType
	status                domain.Status
	category              domain.Category
	chapters, read, fresh int // fresh: chapters found by the last check (shown as new)
	every                 int // days between releases
	tags                  []string
}

func demoSeries() []demo {
	return []demo{
		{"The Lighthouse Keeper's Daughter", "example.org", "A girl keeps the last lighthouse of a drowned coast burning, and learns who it really guides.",
			domain.TypeManga, domain.StatusOngoing, domain.CategoryReading, 48, 46, 2, 7, []string{"drama", "mystery"}},
		{"Iron Orchard", "example.com", "In a city of machines, a gardener grows the only living tree.",
			domain.TypeManhwa, domain.StatusOngoing, domain.CategoryReading, 132, 132, 0, 7, []string{"fantasy"}},
		{"Clockwork Tea House", "example.net", "Every cup of tea at Madame Wen's turns back the clock by one minute.",
			domain.TypeManhua, domain.StatusOngoing, domain.CategoryReading, 87, 60, 0, 3, []string{"slice of life"}},
		{"Starlit Cartographer", "example.org", "A mapmaker charts the islands that only appear at night.",
			domain.TypeNovel, domain.StatusOngoing, domain.CategoryPlanToRead, 210, 0, 0, 2, []string{"adventure"}},
		{"Second Spring Academy", "example.com", "Retired heroes go back to school, as teachers.",
			domain.TypeManhwa, domain.StatusCompleted, domain.CategoryCompleted, 95, 95, 0, 7, []string{"comedy", "action"}},
		{"Moonlit Bakery Chronicles", "example.net", "A bakery that only opens when the moon is full.",
			domain.TypeManga, domain.StatusHiatus, domain.CategoryOnHold, 31, 29, 0, 14, []string{"romance"}},
		{"Echoes of the Ninth Gate", "example.org", "Nine gates, nine worlds, and a courier who must deliver to all of them.",
			domain.TypeManhwa, domain.StatusOngoing, domain.CategoryReading, 64, 63, 1, 7, []string{"action", "fantasy"}},
		{"A Quiet Village Doctor", "example.com", "A city surgeon takes over the clinic of a mountain village.",
			domain.TypeManga, domain.StatusOngoing, domain.CategoryReading, 22, 12, 0, 10, []string{"medical"}},
	}
}

func (d demo) series(i int, now time.Time) domain.Series {
	slug := strings.ReplaceAll(strings.ToLower(strings.Trim(strings.NewReplacer("'", "").Replace(d.title), " ")), " ", "-")
	id := fmt.Sprintf("01JDEMO%019d", i)
	base := "https://" + d.site + "/series/" + slug
	s := domain.Series{
		ID: id, Type: d.typ, Title: d.title, AltTitles: []string{}, Synopsis: d.synopsis, Tags: d.tags,
		Source: domain.Source{URL: base, Domain: d.site, Method: "recipe:" + d.site, Mirrors: []string{}},
		Cover:  domain.Cover{RemoteURL: base + "/cover.png"},
		Status: d.status, StatusRaw: string(d.status), Category: d.category,
		AddedAt:  now.AddDate(0, -2, -i*3),
		Chapters: []domain.Chapter{},
	}
	checked := now.Add(-time.Duration(i+1) * time.Hour)
	s.LastCheckedAt = &checked
	last := now.AddDate(0, 0, -(i % 4)) // the newest release
	for c := d.chapters; c >= 1; c-- {
		n := float64(c)
		ch := domain.Chapter{
			Key: domain.ChapterKey(&n, ""), Number: &n, Label: fmt.Sprintf("Chapter %d", c),
			URL: fmt.Sprintf("%s/chapter-%d", base, c), AltURLs: []string{},
		}
		released := last.AddDate(0, 0, -d.every*(d.chapters-c))
		ch.ReleasedAt = domain.NewDate(released.Year(), released.Month(), released.Day())
		if c > d.chapters-d.fresh {
			ch.DiscoveredAt = &checked
		}
		if c <= d.read {
			at := released.Add(20 * time.Hour)
			ch.Read, ch.ReadAt = true, &at
		}
		s.Chapters = append(s.Chapters, ch)
	}
	seen := float64(d.chapters - d.fresh)
	s.SeenLatestChapter = &seen
	if d.fresh > 0 {
		s.LastNewChapterAt = &checked
	}
	return s
}

// demoCover draws an abstract cover: a two-color gradient with circles.
func demoCover(i int) []byte {
	const w, h = 300, 450
	hue := float64(i) * 47
	top, bottom := hsl(hue, 0.55, 0.62), hsl(hue+40, 0.6, 0.28)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		t := float64(y) / h
		for x := range w {
			c := mix(top, bottom, t)
			for k := range 3 {
				cx, cy := float64(60+((i*53+k*97)%180)), float64(90+((i*71+k*131)%270))
				r := float64(35 + (i*17+k*29)%55)
				if math.Hypot(float64(x)-cx, float64(y)-cy) < r {
					c = mix(c, color.RGBA{255, 255, 255, 255}, 0.18+0.06*float64(k))
				}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // an in-memory image always encodes
	return buf.Bytes()
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

func hsl(h, s, l float64) color.RGBA {
	h = math.Mod(h, 360) / 360
	f := func(n float64) uint8 {
		k := math.Mod(n+h*12, 12)
		a := s * math.Min(l, 1-l)
		return uint8(255 * (l - a*math.Max(-1, math.Min(math.Min(k-3, 9-k), 1))))
	}
	return color.RGBA{f(0), f(8), f(4), 255}
}
