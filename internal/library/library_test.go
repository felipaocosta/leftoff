package library

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/storage"
)

const seriesURL = "https://site.org/manga/x"

// fakeExtractor returns, per URL, the configured result or error.
type fakeExtractor struct {
	mu      sync.Mutex
	results map[string]*extract.Result
	errs    map[string]error
	calls   int
}

func (f *fakeExtractor) Extract(_ context.Context, rawURL string) (*extract.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if err := f.errs[rawURL]; err != nil {
		return nil, err
	}
	r, ok := f.results[rawURL]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeExtractor) set(rawURL string, chapters ...domain.ChapterInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[rawURL] = &extract.Result{
		Series: domain.SeriesInfo{
			URL: rawURL, Title: "X", CoverURL: "https://site.org/c.jpg",
			Status: domain.StatusOngoing, StatusRaw: "OnGoing", Type: domain.TypeManhua,
		},
		Chapters: chapters,
		Method:   "adapter:test",
	}
}

type mutableClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *mutableClock) Now() time.Time  { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *mutableClock) set(t time.Time) { c.mu.Lock(); defer c.mu.Unlock(); c.t = t }

type env struct {
	svc   *Service
	ex    *fakeExtractor
	clock *mutableClock
	dir   string
	store *storage.Store
}

func newEnv(t *testing.T) *env {
	t.Helper()
	clock := &mutableClock{t: t0}
	dir := t.TempDir()
	store, err := storage.Open(context.Background(), dir, clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ex := &fakeExtractor{results: map[string]*extract.Result{}, errs: map[string]error{}}
	ex.set(seriesURL, info(2, seriesURL+"/chapter-2"), info(1, seriesURL+"/chapter-1"))
	return &env{svc: New(store, ex, clock, Options{}), ex: ex, clock: clock, dir: dir, store: store}
}

func (e *env) add(t *testing.T) domain.Series {
	t.Helper()
	s, err := e.svc.Add(context.Background(), seriesURL, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAdd_CreatesSeriesWithDefaults(t *testing.T) {
	e := newEnv(t)
	s := e.add(t)
	if len(s.ID) != 26 || s.Title != "X" || s.Type != domain.TypeManhua || s.Category != domain.CategoryReading ||
		s.Source.Domain != "site.org" || s.Source.Method != "adapter:test" || s.Cover.RemoteURL == "" ||
		!s.AddedAt.Equal(t0) || s.AltTitles == nil || s.Tags == nil || s.Source.Mirrors == nil {
		t.Errorf("series = %+v", s)
	}
	if *s.LatestChapter != 2 || *s.SeenLatestChapter != 2 || s.LastReadChapter != nil {
		t.Errorf("derived: latest=%v seen=%v lastRead=%v", s.LatestChapter, s.SeenLatestChapter, s.LastReadChapter)
	}
	for _, c := range s.Chapters {
		if c.DiscoveredAt != nil || c.Read {
			t.Errorf("imported chapter must be unread with no discovered_at: %+v", c)
		}
	}
}

func TestAdd_Overrides(t *testing.T) {
	e := newEnv(t)
	s, err := e.svc.Add(context.Background(), seriesURL, Overrides{
		Title: "Fixed", Type: domain.TypeManhwa, CoverURL: "https://c.org/x.jpg", Category: domain.CategoryPlanToRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Fixed" || s.Type != domain.TypeManhwa || s.Cover.RemoteURL != "https://c.org/x.jpg" || s.Category != domain.CategoryPlanToRead {
		t.Errorf("overrides not applied: %+v", s)
	}
	for _, bad := range []Overrides{{Type: "comic"}, {Category: "someday"}} {
		if _, err := e.svc.Add(context.Background(), seriesURL, bad); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%+v: err = %v, want ErrInvalidInput", bad, err)
		}
	}
}

func TestAdd_RejectsDuplicates(t *testing.T) {
	e := newEnv(t)
	e.add(t)
	calls := e.ex.calls
	for _, u := range []string{seriesURL, "https://www.site.org/manga/x/", seriesURL + "?utm_source=tw"} {
		if _, err := e.svc.Add(context.Background(), u, Overrides{}); !errors.Is(err, domain.ErrDuplicate) {
			t.Errorf("Add(%s) err = %v, want ErrDuplicate", u, err)
		}
	}
	if e.ex.calls != calls {
		t.Error("duplicates should be rejected before extracting")
	}
	// A different input URL that resolves to the same series page.
	e.ex.results["https://site.org/old-link"] = e.ex.results[seriesURL]
	if _, err := e.svc.Add(context.Background(), "https://site.org/old-link", Overrides{}); !errors.Is(err, domain.ErrDuplicate) {
		t.Errorf("redirected duplicate: err = %v", err)
	}
}

func TestAdd_InvalidURLAndExtractionError(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.Add(context.Background(), "not a url", Overrides{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v", err)
	}
	if _, err := e.svc.Add(context.Background(), "https://site.org/manga/missing", Overrides{}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v", err)
	}
	if len(e.svc.List()) != 0 {
		t.Error("failed adds must not save anything")
	}
}

// TestFlow_AddReadRefreshKeepsState is the Phase 1 acceptance flow at the
// library level: add, mark read, refresh with a new chapter, reopen.
func TestFlow_AddReadRefreshKeepsState(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := e.add(t)
	if _, err := e.svc.MarkReadUpTo(ctx, s.ID, 2); err != nil {
		t.Fatal(err)
	}

	e.clock.set(t1)
	e.ex.set(seriesURL, info(3, seriesURL+"/chapter-3"), info(2, seriesURL+"/chapter-2"), info(1, seriesURL+"/chapter-1"))
	report, err := e.svc.Refresh(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(s.ID)
	if len(report.Added) != 1 || *report.Added[0].Number != 3 || report.Added[0].DiscoveredAt == nil {
		t.Errorf("added = %+v, want only chapter 3", report.Added)
	}
	if report.NewChapters != 1 || *got.LastReadChapter != 2 || *got.LatestChapter != 3 || got.UnreadCount() != 1 {
		t.Errorf("after refresh: report=%+v lastRead=%v latest=%v unread=%d", report, got.LastReadChapter, got.LatestChapter, got.UnreadCount())
	}
	if got.LastNewChapterAt == nil || !got.LastNewChapterAt.Equal(t1) || *got.SeenLatestChapter != 2 {
		t.Errorf("novidades: last_new=%v seen=%v", got.LastNewChapterAt, got.SeenLatestChapter)
	}
	if got.Chapters[0].DiscoveredAt == nil || !got.Chapters[0].DiscoveredAt.Equal(t1) {
		t.Errorf("new chapter discovered_at = %v", got.Chapters[0].DiscoveredAt)
	}

	store, err := storage.Open(ctx, e.dir, e.clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(store, e.ex, e.clock, Options{}).Get(s.ID)
	if *reopened.LastReadChapter != 2 || reopened.UnreadCount() != 1 {
		t.Error("state lost after reopening the library from disk")
	}

	seen, err := e.svc.MarkSeen(ctx, s.ID)
	if err != nil || *seen.SeenLatestChapter != 3 {
		t.Errorf("MarkSeen: %v, seen=%v", err, seen.SeenLatestChapter)
	}
}

func TestRefresh_KeepsUserEditedFields(t *testing.T) {
	e := newEnv(t)
	s, err := e.svc.Add(context.Background(), seriesURL, Overrides{Title: "My title", Type: domain.TypeManhwa})
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.updateSeries(context.Background(), s.ID, func(sr *domain.Series) error {
		sr.Notes, sr.Tags = "note", []string{"fav"}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Refresh(context.Background(), got.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := e.svc.Get(s.ID)
	if after.Title != "My title" || after.Type != domain.TypeManhwa || after.Notes != "note" || len(after.Tags) != 1 {
		t.Errorf("refresh overwrote user fields: %+v", after)
	}
}

func TestRefresh_RecordsFailuresWithoutTouchingChapters(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := e.add(t)
	if _, err := e.svc.MarkRead(ctx, s.ID, []string{"1"}, true); err != nil {
		t.Fatal(err)
	}
	e.ex.errs[seriesURL] = domain.ErrBlocked
	for i := 1; i <= 2; i++ {
		if _, err := e.svc.Refresh(ctx, s.ID); !errors.Is(err, domain.ErrBlocked) {
			t.Fatalf("err = %v, want ErrBlocked", err)
		}
	}
	got, _ := e.svc.Get(s.ID)
	if got.CheckError == nil || got.CheckError.Failures != 2 || len(got.Chapters) != 2 || !got.Chapters[1].Read {
		t.Errorf("after failures: check_error=%+v chapters=%+v", got.CheckError, got.Chapters)
	}

	delete(e.ex.errs, seriesURL)
	e.ex.set(seriesURL) // empty list: refused by the shrink guard
	if _, err := e.svc.Refresh(ctx, s.ID); !errors.Is(err, ErrSourceShrank) {
		t.Fatalf("err = %v, want ErrSourceShrank", err)
	}
	if got, _ := e.svc.Get(s.ID); len(got.Chapters) != 2 || got.CheckError.Failures != 3 {
		t.Errorf("shrunk source changed chapters or did not count: %+v", got)
	}

	e.ex.set(seriesURL, info(2, seriesURL+"/chapter-2"), info(1, seriesURL+"/chapter-1"))
	if _, err := e.svc.Refresh(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(s.ID); got.CheckError != nil {
		t.Error("a successful refresh must clear check_error")
	}
}

func TestRefreshAll_ContinuesAfterFailure(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a := e.add(t)
	other := "https://site.org/manga/y"
	e.ex.set(other, info(1, other+"/chapter-1"))
	b, err := e.svc.Add(ctx, other, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	e.ex.errs[seriesURL] = errors.New("boom")
	reports := e.svc.RefreshAll(ctx)
	if len(reports) != 2 || reports[0].SeriesID != a.ID || reports[0].Err == nil || reports[1].SeriesID != b.ID || reports[1].Err != nil {
		t.Errorf("reports = %+v", reports)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, r := range e.svc.RefreshAll(canceled) {
		if !errors.Is(r.Err, context.Canceled) {
			t.Errorf("canceled report = %+v", r)
		}
	}
}

func TestMarkRead(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := e.add(t)

	got, err := e.svc.MarkRead(ctx, s.ID, []string{"1", "2"}, true)
	if err != nil || got.UnreadCount() != 0 || !got.Chapters[0].ReadAt.Equal(t0) {
		t.Fatalf("mark read: %v %+v", err, got.Chapters)
	}
	e.clock.set(t1)
	got, _ = e.svc.MarkRead(ctx, s.ID, []string{"2"}, true)
	if !got.Chapters[0].ReadAt.Equal(t0) {
		t.Error("re-marking must keep the original read_at")
	}
	got, _ = e.svc.MarkRead(ctx, s.ID, []string{"2"}, false)
	if got.Chapters[0].Read || got.Chapters[0].ReadAt != nil || *got.LastReadChapter != 1 {
		t.Errorf("unmark: %+v lastRead=%v", got.Chapters[0], got.LastReadChapter)
	}
	if _, err := e.svc.MarkRead(ctx, s.ID, []string{"1", "nope"}, false); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown key: err = %v", err)
	}
	if got, _ := e.svc.Get(s.ID); !got.Chapters[1].Read {
		t.Error("a failed MarkRead must change nothing")
	}
}

func TestMarkReadUpTo_SkipsChaptersWithoutNumber(t *testing.T) {
	e := newEnv(t)
	e.ex.set(seriesURL, info(2, seriesURL+"/c2"), domain.ChapterInfo{Label: "Extra", URL: seriesURL + "/extra"}, info(1, seriesURL+"/c1"))
	s := e.add(t)
	got, err := e.svc.MarkReadUpTo(context.Background(), s.ID, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Chapters[0].Read || got.Chapters[1].Read || !got.Chapters[2].Read {
		t.Errorf("chapters = %+v", got.Chapters)
	}
}

func TestRemoveAndNotFound(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s := e.add(t)
	if err := e.svc.Remove(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Get(s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after remove: %v", err)
	}
	for name, err := range map[string]error{
		"remove":  e.svc.Remove(ctx, s.ID),
		"refresh": second(e.svc.Refresh(ctx, s.ID)),
		"read":    second(e.svc.MarkRead(ctx, s.ID, nil, true)),
		"seen":    second(e.svc.MarkSeen(ctx, s.ID)),
	} {
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestPreview_DoesNotSave(t *testing.T) {
	e := newEnv(t)
	res, err := e.svc.Preview(context.Background(), seriesURL)
	if err != nil || res.Series.Title != "X" || len(e.svc.List()) != 0 {
		t.Errorf("preview: %v %+v", err, res)
	}
}

func second[T any](_ T, err error) error { return err }

func TestRefresh_RecordsTheKindOfFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"javascript page, no browser", fmt.Errorf("x: %w: %w", extract.ErrNeedsBrowser, extract.ErrIncomplete), domain.CheckNeedsBrowser},
		{"cloudflare, no browser", fmt.Errorf("x: %w", fetch.ErrNoBrowser), domain.CheckNoBrowser},
		{"site down", fmt.Errorf("x: %w: refused", fetch.ErrUnreachable), domain.CheckUnreachable},
		{"site error", &fetch.StatusError{URL: "u", StatusCode: 500}, domain.CheckUnreachable},
		{"timeout", fmt.Errorf("x: %w: %w", fetch.ErrUnreachable, context.DeadlineExceeded), domain.CheckTimeout},
		{"anything else", errors.New("boom"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			s := e.add(t)
			e.ex.errs[seriesURL] = tt.err
			if _, err := e.svc.Refresh(context.Background(), s.ID); err == nil {
				t.Fatal("refresh should fail")
			}
			got, _ := e.svc.Get(s.ID)
			if got.CheckError == nil || got.CheckError.Code != tt.want {
				t.Errorf("check_error = %+v, want code %q", got.CheckError, tt.want)
			}
		})
	}
}
