package library

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
	"github.com/felipaocosta/leftoff/internal/fetch"
	"github.com/felipaocosta/leftoff/internal/storage"
)

var pngCover = []byte("\x89PNG\r\n\x1a\n" + "cover")

// imageFetcher serves images by URL and records the Referer of each request.
type imageFetcher struct {
	mu       sync.Mutex
	images   map[string][]byte
	referers []string
}

func (f *imageFetcher) Fetch(_ context.Context, req *fetch.Request) (*fetch.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.referers = append(f.referers, req.Header.Get("Referer"))
	body, ok := f.images[req.URL]
	if !ok {
		return nil, domain.ErrNotFound
	}
	u, _ := url.Parse(req.URL)
	return &fetch.Response{URL: u, StatusCode: http.StatusOK, Body: body}, nil
}

func newCoverEnv(t *testing.T) (*env, *imageFetcher) {
	t.Helper()
	e := newEnv(t)
	store, err := storage.Open(context.Background(), e.dir, e.clock, storage.Options{})
	if err != nil {
		t.Fatal(err)
	}
	imgs := &imageFetcher{images: map[string][]byte{"https://site.org/c.jpg": pngCover}}
	e.svc = New(store, e.ex, e.clock, Options{Fetcher: imgs})
	return e, imgs
}

func TestCover_DownloadedOnAddAndDeletedOnRemove(t *testing.T) {
	e, imgs := newCoverEnv(t)
	s := e.add(t)
	want := "covers/" + s.ID + ".png"
	if s.Cover.Local != want {
		t.Fatalf("cover.local = %q, want %q", s.Cover.Local, want)
	}
	if len(imgs.referers) != 1 || imgs.referers[0] != seriesURL {
		t.Errorf("referers = %v, want the series page", imgs.referers)
	}
	path := filepath.Join(e.dir, filepath.FromSlash(want))
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Remove(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("cover file left behind after remove")
	}
}

func TestCover_FailureDoesNotFailAddAndIsRetriedOnRefresh(t *testing.T) {
	e, imgs := newCoverEnv(t)
	delete(imgs.images, "https://site.org/c.jpg")
	s := e.add(t)
	if s.Cover.Local != "" {
		t.Fatalf("cover.local = %q, want empty after failure", s.Cover.Local)
	}

	imgs.images["https://site.org/c.jpg"] = []byte("<html>blocked</html>") // not an image
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(s.ID); got.Cover.Local != "" {
		t.Error("a non-image must not become the cover")
	}

	imgs.images["https://site.org/c.jpg"] = pngCover
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(s.ID); got.Cover.Local == "" {
		t.Error("refresh should retry a missing cover")
	}
}

func TestCover_RefreshDownloadsOnlyWhenRemoteChanges(t *testing.T) {
	e, imgs := newCoverEnv(t)
	s := e.add(t)
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if len(imgs.referers) != 1 {
		t.Errorf("unchanged cover was downloaded again: %d requests", len(imgs.referers))
	}

	e.ex.set(seriesURL, info(2, seriesURL+"/chapter-2"), info(1, seriesURL+"/chapter-1"))
	e.ex.results[seriesURL].Series.CoverURL = "https://site.org/new.jpg"
	imgs.images["https://site.org/new.jpg"] = pngCover
	if _, err := e.svc.Refresh(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(s.ID)
	if len(imgs.referers) != 2 || got.Cover.RemoteURL != "https://site.org/new.jpg" || got.Cover.Local == "" {
		t.Errorf("requests=%d cover=%+v", len(imgs.referers), got.Cover)
	}
}

func TestCover_DisabledWithoutFetcher(t *testing.T) {
	e := newEnv(t) // Options{} has no fetcher
	if s := e.add(t); s.Cover.Local != "" || s.Cover.RemoteURL == "" {
		t.Errorf("cover = %+v", s.Cover)
	}
}

func TestUploadCover_IsKeptByRefreshUntilRefreshCover(t *testing.T) {
	e, imgs := newCoverEnv(t)
	ctx := context.Background()
	s := e.add(t)
	jpeg := []byte("\xff\xd8\xff\xe0mine")

	got, err := e.svc.UploadCover(ctx, s.ID, jpeg)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Cover.Custom || got.Cover.Local != "covers/"+s.ID+".jpg" || got.Cover.UpdatedAt == nil {
		t.Fatalf("after upload: %+v", got.Cover)
	}
	uploadedAt := *got.Cover.UpdatedAt

	// The site now has another cover, but a normal refresh keeps the upload.
	e.ex.set(seriesURL, info(2, seriesURL+"/chapter-2"), info(1, seriesURL+"/chapter-1"))
	e.ex.results[seriesURL].Series.CoverURL = "https://site.org/new.jpg"
	imgs.images["https://site.org/new.jpg"] = pngCover
	if _, err := e.svc.Refresh(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.svc.Get(s.ID); !got.Cover.Custom || got.Cover.Local != "covers/"+s.ID+".jpg" || got.Cover.RemoteURL != "https://site.org/c.jpg" {
		t.Errorf("refresh replaced the uploaded cover: %+v", got.Cover)
	}

	// "Refresh cover" goes back to the site's current cover.
	e.clock.set(t1)
	got, err = e.svc.RefreshCover(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cover.UpdatedAt == nil || !got.Cover.UpdatedAt.After(uploadedAt) {
		t.Errorf("cover.updated_at must change when the file is replaced: %v → %v", uploadedAt, got.Cover.UpdatedAt)
	}
	if got.Cover.Custom || got.Cover.RemoteURL != "https://site.org/new.jpg" || got.Cover.Local != "covers/"+s.ID+".png" {
		t.Errorf("after RefreshCover: %+v", got.Cover)
	}
}

func TestRefreshCover_DownloadsAgainEvenIfTheURLIsTheSame(t *testing.T) {
	e, imgs := newCoverEnv(t)
	s := e.add(t)
	before := len(imgs.referers)
	if _, err := e.svc.RefreshCover(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if len(imgs.referers) != before+1 {
		t.Errorf("cover requests %d → %d, want one more", before, len(imgs.referers))
	}
}

func TestCoverActions_Errors(t *testing.T) {
	e, imgs := newCoverEnv(t)
	ctx := context.Background()
	s := e.add(t)
	if _, err := e.svc.UploadCover(ctx, s.ID, []byte("<html>")); !errors.Is(err, storage.ErrNotImage) {
		t.Errorf("non-image upload: %v", err)
	}
	if _, err := e.svc.UploadCover(ctx, "missing", pngCover); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing series: %v", err)
	}
	e.ex.results[seriesURL].Series.CoverURL = ""
	if _, err := e.svc.RefreshCover(ctx, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("site without cover: %v", err)
	}
	e.ex.results[seriesURL].Series.CoverURL = "https://site.org/gone.jpg"
	delete(imgs.images, "https://site.org/gone.jpg")
	if _, err := e.svc.RefreshCover(ctx, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("download failure must be reported: %v", err)
	}
	plain := newEnv(t) // no fetcher: covers disabled
	ps := plain.add(t)
	if _, err := plain.svc.RefreshCover(ctx, ps.ID); err == nil {
		t.Error("RefreshCover without a fetcher should fail")
	}
}
