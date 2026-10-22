//go:build perf

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// The performance check (RNF-01, RNF-03), outside the normal go test:
//
//	go test -tags perf -run TestPerf -v ./internal/app
//
// It writes a realistic library of 500 series (PERF_DATA_DIR keeps it, to
// measure the real process with ./bin/server; PERF_MAX_CHAPTERS=180 makes
// a lighter one) and measures opening it,
// the memory it takes and the API answers of the real server.
const perfSeries = 500

func perfLibrary(t *testing.T, dir string) (chapters int) {
	t.Helper()
	maxChapters := 420 // PERF_MAX_CHAPTERS: the longest regular series
	if v, err := strconv.Atoi(os.Getenv("PERF_MAX_CHAPTERS")); err == nil && v > 20 {
		maxChapters = v
	}
	r := rand.New(rand.NewPCG(1, 2)) // the same library every run
	clock := domain.FixedClock{T: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)}
	store, err := storage.Open(context.Background(), dir, clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	synopsis := strings.Repeat("A long synopsis about a hunter who regresses. ", 12)
	err = store.Update(context.Background(), func(l *storage.Library) error {
		for i := range perfSeries {
			n := 20 + r.IntN(maxChapters-20) // about 250 on average by default
			if i%50 == 0 {
				n = 1500 // a few very long ones
			}
			read := r.IntN(n + 1)
			s := domain.Series{
				ID: fmt.Sprintf("01JPERF%019d", i), Type: domain.TypeManhwa, Title: fmt.Sprintf("Series %03d", i),
				AltTitles: []string{fmt.Sprintf("Alt %d", i)},
				Source: domain.Source{URL: fmt.Sprintf("https://site%d.org/manga/series-%d/", i%12, i), Domain: fmt.Sprintf("site%d.org", i%12),
					Method: "adapter:madara", Mirrors: []string{}},
				Cover:  domain.Cover{RemoteURL: "https://site.org/c.jpg"},
				Status: domain.StatusOngoing, StatusRaw: "Ongoing", Synopsis: synopsis, Tags: []string{"action"},
				Category: domain.CategoryReading, AddedAt: clock.T.Add(-time.Duration(i) * time.Hour),
			}
			for c := n; c >= 1; c-- {
				num := float64(c)
				ch := domain.Chapter{
					Key: domain.ChapterKey(&num, ""), Number: &num, Label: fmt.Sprintf("Chapter %d", c),
					URL:        fmt.Sprintf("https://site%d.org/manga/series-%d/chapter-%d/", i%12, i, c),
					AltURLs:    []string{},
					ReleasedAt: domain.NewDate(2020, time.January, 1+c*7),
				}
				if c <= read {
					at := clock.T.Add(-time.Duration(n-c) * time.Hour)
					ch.Read, ch.ReadAt = true, &at
				}
				s.Chapters = append(s.Chapters, ch)
			}
			chapters += n
			l.Series = append(l.Series, s)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return chapters
}

func heapMB() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapInuse) / (1 << 20)
}

// timed runs f n times and returns the median duration.
func timed(n int, f func()) time.Duration {
	var ds []time.Duration
	for range n {
		start := time.Now()
		f()
		ds = append(ds, time.Since(start))
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return ds[len(ds)/2].Round(100 * time.Microsecond)
}

func TestPerf(t *testing.T) {
	dir := os.Getenv("PERF_DATA_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	chapters := perfLibrary(t, dir)
	info, err := os.Stat(filepath.Join(dir, "library.json"))
	if err != nil {
		t.Fatal(err)
	}

	before := heapMB()
	var store *storage.Store
	open := timed(3, func() {
		store, err = storage.Open(context.Background(), dir, domain.SystemClock{}, storage.Options{})
		if err != nil {
			t.Fatal(err)
		}
	})
	heap := heapMB() - before
	runtime.KeepAlive(store)

	srv := startServer(t, Config{DataDir: dir}, newWorld(), ServeOptions{})
	var list struct {
		Series []json.RawMessage `json:"series"`
	}
	listTime := timed(5, func() { srv.call("GET", "/series", nil, &list) })
	if len(list.Series) != perfSeries {
		t.Fatalf("series = %d", len(list.Series))
	}
	search := timed(5, func() { srv.call("GET", "/series?q=series%2042&sort=unread", nil, nil) })
	biggest := fmt.Sprintf("01JPERF%019d", 0)
	detail := timed(5, func() { srv.call("GET", "/series/"+biggest, nil, nil) })
	mark := 0
	write := timed(5, func() {
		mark++
		srv.call("POST", "/series/"+biggest+"/read", map[string]any{"keys": []string{fmt.Sprint(mark)}, "read": true}, nil)
	})
	_, listBody := srv.raw("GET", "/series", nil)

	t.Logf("\n| Medida | Valor | Meta |\n|---|---|---|\n"+
		"| Biblioteca | %d obras, %d capítulos, library.json de %.1f MB | — |\n"+
		"| Abrir a biblioteca (storage.Open) | %v | — |\n"+
		"| Memória da biblioteca aberta (heap) | %.1f MB | RNF-01: servidor ocioso < 50 MB |\n"+
		"| GET /series (500 obras, %.0f KB) | %v | RNF-03: < 1 s |\n"+
		"| GET /series?q=…&sort=unread | %v | — |\n"+
		"| GET /series/{id} (1500 capítulos) | %v | — |\n"+
		"| Marcar um capítulo (grava a biblioteca) | %v | — |",
		perfSeries, chapters, float64(info.Size())/(1<<20), open, heap,
		float64(len(listBody))/1024, listTime, search, detail, write)

	if listTime > time.Second {
		t.Errorf("RNF-03: GET /series took %v", listTime)
	}
}
