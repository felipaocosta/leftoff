package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// FuzzOpen_LibraryFile: any library.json either fails to open (and is left
// untouched) or opens into a library that saves and reopens unchanged.
func FuzzOpen_LibraryFile(f *testing.F) {
	for _, name := range []string{"library-v1.json"} {
		data, err := testfixture.ReadFile(f, filepath.Join("..", "..", "testdata", "storage", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte(`{"schema":2,"series":[{"id":"a","chapters":[{"key":"1","number":1,"read":true}]}]}`))
	f.Add([]byte(`{"schema":1,"series":null}`))
	f.Add([]byte(`{"schema":9}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, libraryFile)
		if err := os.WriteFile(path, data, filePerm); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		s, err := Open(ctx, dir, domain.FixedClock{T: t0}, Options{})
		if err != nil {
			if got, _ := os.ReadFile(path); !bytes.Equal(got, data) {
				t.Fatal("a library that failed to open was changed")
			}
			return
		}
		// Save (an empty update rewrites the file), reopen, compare.
		if err := s.Update(ctx, func(*Library) error { return nil }); err != nil {
			t.Fatalf("update: %v", err)
		}
		first, _ := encodeLibrary(s.Library())
		again, err := Open(ctx, dir, domain.FixedClock{T: t0}, Options{})
		if err != nil {
			t.Fatalf("reopen a saved library: %v\n%s", err, first)
		}
		second, _ := encodeLibrary(again.Library())
		if !bytes.Equal(first, second) {
			t.Fatalf("not stable across save and reopen:\n%s\n%s", first, second)
		}
		if err := s.Update(ctx, func(*Library) error { return nil }); err != nil {
			t.Fatalf("second update: %v", err)
		}
	})
}

// FuzzOpen_SettingsFile: any settings.json fails cleanly or round-trips,
// unknown fields included.
func FuzzOpen_SettingsFile(f *testing.F) {
	f.Add([]byte(`{"schema":1,"check_interval_hours":6,"notifications":{"ntfy_url":"https://ntfy.sh/x","x":1},"ui":{"theme":"dark"}}`))
	f.Add([]byte(`{"schema":1,"desktop":{"mode":"connected","server_url":"http://h:1"},"enrichment":{"enabled":true}}`))
	f.Add([]byte(`{"schema":2}`))
	f.Add([]byte(`{"notifications":null}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, settingsFile), data, filePerm); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		s, err := Open(ctx, dir, domain.FixedClock{T: t0}, Options{})
		if err != nil {
			return
		}
		if err := s.SaveSettings(ctx, s.Settings()); err != nil {
			t.Fatalf("save: %v", err)
		}
		first, _ := json.Marshal(s.Settings())
		again, err := Open(ctx, dir, domain.FixedClock{T: t0}, Options{})
		if err != nil {
			t.Fatalf("reopen saved settings: %v\n%s", err, first)
		}
		if second, _ := json.Marshal(again.Settings()); !bytes.Equal(first, second) {
			t.Fatalf("settings not stable:\n%s\n%s", first, second)
		}
	})
}
