package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// library-v1-edge.json was written by the storage code of schema 1 (before
// be4730d): chapters without a number, half chapters with alt URLs and
// unknown fields, a custom cover, a check error, mirrors, an empty series.

func decodeGeneric(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestMigrateFromV1_KeepsEveryFieldOfARealV1File(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "library-v1-edge.json", dir)
	original, err := os.ReadFile(filepath.Join(dir, libraryFile))
	if err != nil {
		t.Fatal(err)
	}
	openStore(t, dir, domain.FixedClock{T: t0})
	migrated, err := os.ReadFile(filepath.Join(dir, libraryFile))
	if err != nil {
		t.Fatal(err)
	}

	want, got := decodeGeneric(t, original), decodeGeneric(t, migrated)
	if got["schema"] != float64(CurrentSchema) {
		t.Fatalf("schema = %v", got["schema"])
	}
	wantSeries, gotSeries := want["series"].([]any), got["series"].([]any)
	if len(gotSeries) != len(wantSeries) {
		t.Fatalf("series = %d, want %d", len(gotSeries), len(wantSeries))
	}
	for i := range wantSeries {
		w, g := wantSeries[i].(map[string]any), gotSeries[i].(map[string]any)
		if ext, ok := g["external"]; !ok || ext != nil {
			t.Errorf("series %d: external = %v, want null", i, ext)
		}
		delete(g, "external")
		if ce, ok := g["check_error"].(map[string]any); ok { // v3 adds the code
			if ce["code"] != "" {
				t.Errorf("series %d: check_error.code = %v, want \"\" for %q", i, ce["code"], ce["message"])
			}
			delete(ce, "code")
		}
		if !reflect.DeepEqual(w, g) {
			wj, _ := json.MarshalIndent(w, "", " ")
			gj, _ := json.MarshalIndent(g, "", " ")
			t.Errorf("series %d changed by the migration:\nwant %s\ngot  %s", i, wj, gj)
		}
	}
	backups, _ := os.ReadDir(filepath.Join(dir, backupsDir))
	if len(backups) != 1 {
		t.Fatalf("backups = %d, want 1 before the migration", len(backups))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, backupsDir, backups[0].Name())); string(b) != string(original) {
		t.Error("the backup is not the original v1 file")
	}
}

func TestOpen_IgnoresATempFileLeftByACrash(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, domain.FixedClock{T: t0})
	if err := s.Update(context.Background(), addSeries("a")); err != nil {
		t.Fatal(err)
	}
	// A crash between writing the temp file and the rename leaves half a file.
	stale := filepath.Join(dir, libraryFile+".123.tmp")
	if err := os.WriteFile(stale, []byte(`{"schema":2,"series":[{"id":"b"`), filePerm); err != nil {
		t.Fatal(err)
	}
	again := openStore(t, dir, domain.FixedClock{T: t0})
	if lib := again.Library(); len(lib.Series) != 1 || lib.Series[0].ID != "a" {
		t.Errorf("series = %+v", lib.Series)
	}
	if err := again.Update(context.Background(), addSeries("c")); err != nil {
		t.Fatalf("write after a crash: %v", err)
	}
}

func TestOpen_RemovesOldTempFilesOnly(t *testing.T) {
	dir := t.TempDir()
	openStore(t, dir, domain.FixedClock{T: t0}) // creates the directories
	files := map[string]time.Time{
		libraryFile + ".1.tmp":                          t0.Add(-2 * time.Hour), // left by a crash
		settingsFile + ".2.tmp":                         t0.Add(-25 * time.Hour),
		filepath.Join(coversDir, "a.jpg.3.tmp"):         t0.Add(-2 * time.Hour),
		filepath.Join("recipes", "site.org.json.4.tmp"): t0.Add(-2 * time.Hour),
		libraryFile + ".5.tmp":                          t0.Add(-time.Minute),    // a write in progress
		"notes.tmp":                                     t0.Add(-48 * time.Hour), // not ours
	}
	_ = os.MkdirAll(filepath.Join(dir, "recipes"), dirPerm) // may already exist
	for name, mtime := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), filePerm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	openStore(t, dir, domain.FixedClock{T: t0})
	for name, mtime := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		keep := t0.Sub(mtime) < staleTempAge || name == "notes.tmp"
		if exists := err == nil; exists != keep {
			t.Errorf("%s: exists = %v, want %v", name, exists, keep)
		}
	}
}
