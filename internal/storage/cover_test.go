package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

var (
	pngData  = []byte("\x89PNG\r\n\x1a\n" + "rest-of-png")
	jpegData = []byte("\xff\xd8\xff\xe0" + "rest-of-jpeg")
)

func TestCovers_SaveReadReplaceDelete(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, domain.FixedClock{T: t0})
	ctx := context.Background()

	rel, err := s.SaveCover(ctx, "01JABC", pngData)
	if err != nil || rel != "covers/01JABC.png" {
		t.Fatalf("SaveCover = %q, %v", rel, err)
	}
	data, ct, err := s.ReadCover("01JABC")
	if err != nil || ct != "image/png" || string(data) != string(pngData) {
		t.Fatalf("ReadCover = %q, %q, %v", data, ct, err)
	}

	// A new cover in another format replaces the old file.
	if rel, err = s.SaveCover(ctx, "01JABC", jpegData); err != nil || rel != "covers/01JABC.jpg" {
		t.Fatalf("replace = %q, %v", rel, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "covers", "01JABC.png")); !errors.Is(err, os.ErrNotExist) {
		t.Error("old png cover was not removed")
	}
	if _, ct, _ := s.ReadCover("01JABC"); ct != "image/jpeg" {
		t.Errorf("content type = %q", ct)
	}

	if err := s.DeleteCover(ctx, "01JABC"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReadCover("01JABC"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after delete: err = %v", err)
	}
	if err := s.DeleteCover(ctx, "01JABC"); err != nil {
		t.Errorf("deleting a missing cover should succeed: %v", err)
	}
}

func TestCovers_RejectsBadInput(t *testing.T) {
	s := openStore(t, t.TempDir(), domain.FixedClock{T: t0})
	ctx := context.Background()
	if _, err := s.SaveCover(ctx, "x", []byte("<html>not an image</html>")); !errors.Is(err, ErrNotImage) {
		t.Errorf("html: err = %v, want ErrNotImage", err)
	}
	for _, id := range []string{"../library", "a/b", ""} {
		if _, err := s.SaveCover(ctx, id, pngData); err == nil {
			t.Errorf("SaveCover(%q) should fail", id)
		}
		if _, _, err := s.ReadCover(id); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("ReadCover(%q) err = %v", id, err)
		}
		if err := s.DeleteCover(ctx, id); err != nil {
			t.Errorf("DeleteCover(%q) err = %v", id, err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.SaveCover(canceled, "x", pngData); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled save: %v", err)
	}
	if err := s.DeleteCover(canceled, "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled delete: %v", err)
	}
}

func TestRecipes_SaveListDelete(t *testing.T) {
	s := openStore(t, t.TempDir(), domain.FixedClock{T: t0})
	ctx := context.Background()
	if got, err := s.Recipes(); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	if err := s.SaveRecipe(ctx, "site.org", []byte(`{"schema":1}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Recipes()
	if err != nil || string(got["site.org"]) != `{"schema":1}` {
		t.Errorf("recipes = %v %v", got, err)
	}
	for _, bad := range []string{"../x", "Site.org", ""} {
		if err := s.SaveRecipe(ctx, bad, nil); !errors.Is(err, ErrBadRecipeName) {
			t.Errorf("SaveRecipe(%q) = %v", bad, err)
		}
	}
	if err := s.DeleteRecipe(ctx, "site.org"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRecipe(ctx, "site.org"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("deleting again: %v", err)
	}
}
