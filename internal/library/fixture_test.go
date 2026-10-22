package library

import (
	"context"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/extract"
	"github.com/felipaocosta/leftoff/internal/extract/adapters/mangastream"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/storage"
)

// TestFixture_RealAdapterAddReadRefresh runs the real pipeline and adapter
// on the saved ravenscans page: add, mark up to 100, refresh unchanged.
func TestFixture_RealAdapterAddReadRefresh(t *testing.T) {
	const u = "https://ravenscans.org/manga/the-extras-academy-survival-guide/"
	html, err := testfixture.ReadFile(t, "../../testdata/fixtures/ravenscans.org/the-extras-academy-survival-guide.html")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	clock := &mutableClock{t: t0}
	store, err := storage.Open(ctx, t.TempDir(), clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ex := extract.New(&fetch.Static{Pages: map[string][]byte{u: html}},
		extract.Options{Adapters: []extract.Adapter{mangastream.New()}})
	svc := New(store, ex, clock, Options{})

	s, err := svc.Add(ctx, u, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkReadUpTo(ctx, s.ID, 100); err != nil {
		t.Fatal(err)
	}
	clock.set(t1)
	report, err := svc.Refresh(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(s.ID)
	if report.NewChapters != 0 || len(got.Chapters) != 130 || *got.LastReadChapter != 100 || got.UnreadCount() != 28 {
		t.Errorf("new=%d chapters=%d lastRead=%v unread=%d", report.NewChapters, len(got.Chapters), got.LastReadChapter, got.UnreadCount())
	}
	if got.ReleaseIntervalDays == nil || *got.ReleaseIntervalDays != 7 || got.NextReleaseEstimate.String() != "2026-10-22" {
		t.Errorf("release estimate: interval=%v next=%q", got.ReleaseIntervalDays, got.NextReleaseEstimate)
	}
}
