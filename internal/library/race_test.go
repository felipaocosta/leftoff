package library

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// blockingExtractor holds every extraction until release is closed.
type blockingExtractor struct {
	inner   *fakeExtractor
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingExtractor) Extract(ctx context.Context, rawURL string) (*extract.Result, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return b.inner.Extract(ctx, rawURL)
}

func TestRefresh_KeepsChaptersMarkedReadWhileItWasFetching(t *testing.T) {
	e := newEnv(t)
	s := e.add(t)
	ctx := context.Background()
	slow := &blockingExtractor{inner: e.ex, started: make(chan struct{}), release: make(chan struct{})}
	svc := New(e.store, slow, e.clock, Options{})
	e.ex.set(seriesURL, info(3, seriesURL+"/chapter-3"), info(2, seriesURL+"/chapter-2"), info(1, seriesURL+"/chapter-1"))

	done := make(chan error, 1)
	go func() { _, err := svc.Refresh(ctx, s.ID); done <- err }()
	<-slow.started
	if _, err := svc.MarkReadUpTo(ctx, s.ID, 2); err != nil { // while the page downloads
		t.Fatal(err)
	}
	close(slow.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(s.ID)
	m := byKey(t, got.Chapters)
	if !m["1"].Read || !m["2"].Read || m["3"].Read || len(m) != 3 {
		t.Errorf("chapters = %+v", got.Chapters)
	}
}

func TestConcurrentReadsAndRefreshes_LoseNothing(t *testing.T) {
	e := newEnv(t)
	s := e.add(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = e.svc.Refresh(ctx, s.ID) }() // result checked below
		go func() {
			defer wg.Done()
			if _, err := e.svc.MarkRead(ctx, s.ID, []string{"1"}, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _ := e.svc.Get(s.ID)
	if m := byKey(t, got.Chapters); !m["1"].Read {
		t.Errorf("chapter 1 lost its read mark: %+v", got.Chapters)
	}
}

// openEdge opens a library from testdata/storage/library-v1-edge.json.
func openEdge(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	data, err := testfixture.ReadFile(t, filepath.Join("..", "..", "testdata", "storage", "library-v1-edge.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "library.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	clock := &mutableClock{t: t0}
	store, err := storage.Open(context.Background(), dir, clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return New(store, &fakeExtractor{results: map[string]*extract.Result{}, errs: map[string]error{}}, clock, Options{})
}

func TestExportImport_RoundTripOfAMigratedV1Library(t *testing.T) {
	src := openEdge(t)
	data, err := json.Marshal(src.Export())
	if err != nil {
		t.Fatal(err)
	}
	dst := newEnv(t)
	report, err := dst.svc.Import(context.Background(), data)
	if err != nil || report.Added != 3 {
		t.Fatalf("import = %+v, %v", report, err)
	}
	for _, want := range src.List() {
		got, err := dst.svc.Get(want.ID)
		if err != nil {
			t.Fatalf("%s missing: %v", want.Title, err)
		}
		w, g := readMarks(want.Chapters), readMarks(got.Chapters)
		if len(w) != len(g) {
			t.Errorf("%s: read %v, want %v", want.Title, g, w)
		}
		for k, at := range w {
			if g[k] != at {
				t.Errorf("%s chapter %s: read_at %q, want %q", want.Title, k, g[k], at)
			}
		}
		if got.Notes != want.Notes || len(got.Tags) != len(want.Tags) || got.Category != want.Category {
			t.Errorf("%s: user fields changed: %+v", want.Title, got)
		}
	}
}

func readMarks(chs []domain.Chapter) map[string]string {
	m := map[string]string{}
	for _, c := range chs {
		if c.Read {
			m[c.Key] = ""
			if c.ReadAt != nil {
				m[c.Key] = c.ReadAt.String()
			}
		}
	}
	return m
}
