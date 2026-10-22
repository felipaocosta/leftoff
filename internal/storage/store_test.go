package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"

	"github.com/felipaocosta/leftoff/internal/domain"
)

var t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// manualClock is a Clock whose time is advanced by the test.
type manualClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func openStore(t *testing.T, dir string, clock domain.Clock) *Store {
	t.Helper()
	s, err := Open(context.Background(), dir, clock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func copyFixture(t *testing.T, name, dir string) {
	t.Helper()
	data, err := testfixture.ReadFile(t, filepath.Join("..", "..", "testdata", "storage", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, libraryFile), data, filePerm); err != nil {
		t.Fatal(err)
	}
}

func addSeries(id string) func(*Library) error {
	return func(l *Library) error {
		l.Series = append(l.Series, domain.Series{ID: id, Title: "T " + id})
		return nil
	}
}

func TestOpen_EmptyDirStartsWithEmptyLibrary(t *testing.T) {
	s := openStore(t, t.TempDir(), domain.FixedClock{T: t0})
	lib := s.Library()
	if lib.Schema != CurrentSchema || lib.Series == nil || len(lib.Series) != 0 {
		t.Errorf("got %+v, want empty library at current schema", lib)
	}
}

func TestUpdate_PersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, domain.FixedClock{T: t0})
	if err := s.Update(context.Background(), addSeries("a")); err != nil {
		t.Fatal(err)
	}
	reopened := openStore(t, dir, domain.FixedClock{T: t0})
	lib := reopened.Library()
	if len(lib.Series) != 1 || lib.Series[0].ID != "a" {
		t.Fatalf("series = %+v, want [a]", lib.Series)
	}
	if !lib.UpdatedAt.Equal(t0) {
		t.Errorf("updated_at = %v, want %v", lib.UpdatedAt, t0)
	}
}

func TestStore_PreservesV1FileReadStateAndUnknownFields(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "library-v1.json", dir)
	s := openStore(t, dir, domain.FixedClock{T: t0})

	err := s.Update(context.Background(), func(l *Library) error {
		l.Series[0].Notes = "edited"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, libraryFile))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"sync_token": "abc"`, `"rating_from_future_version": 5`, `"bookmark_page": 12`,
		`"notes": "edited"`, `"read_at": "2026-10-12T20:00:00Z"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved file lacks %s", want)
		}
	}
	ch := openStore(t, dir, domain.FixedClock{T: t0}).Library().Series[0].Chapters[1]
	if ch.Key != "244" || !ch.Read {
		t.Errorf("read state lost: %+v", ch)
	}
}

func TestUpdate_FnErrorChangesNothing(t *testing.T) {
	dir := t.TempDir()
	s := openStore(t, dir, domain.FixedClock{T: t0})
	if err := s.Update(context.Background(), addSeries("a")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, libraryFile))
	boom := errors.New("boom")
	err := s.Update(context.Background(), func(l *Library) error {
		l.Series[0].Title = "changed"
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, libraryFile))
	if s.Library().Series[0].Title != "T a" || string(before) != string(after) {
		t.Error("failed update leaked into snapshot or file")
	}
}

func TestUpdate_RecomputesDerivedFields(t *testing.T) {
	s := openStore(t, t.TempDir(), domain.FixedClock{T: t0})
	one, two := 1.0, 2.0
	err := s.Update(context.Background(), func(l *Library) error {
		l.Series = append(l.Series, domain.Series{ID: "a", Chapters: []domain.Chapter{
			{Key: "1", Number: &one, Read: true}, {Key: "2", Number: &two},
		}})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := s.Library().Series[0]
	if got.LastReadChapter == nil || *got.LastReadChapter != 1 || *got.LatestChapter != 2 {
		t.Errorf("derived fields not recomputed: last_read=%v latest=%v", got.LastReadChapter, got.LatestChapter)
	}
}

func TestUpdate_ConcurrentWritersAreAllApplied(t *testing.T) {
	s := openStore(t, t.TempDir(), domain.FixedClock{T: t0})
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.Update(context.Background(), addSeries(string(rune('a'+i)))); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := len(s.Library().Series); n != 20 {
		t.Errorf("got %d series, want 20", n)
	}
}

func TestUpdate_CanceledContext(t *testing.T) {
	s := openStore(t, t.TempDir(), domain.FixedClock{T: t0})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Update(ctx, addSeries("a")); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestOpen_RejectsBadFilesWithoutTouchingThem(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr error
	}{
		{"newer schema", `{"schema": 99, "series": []}`, ErrNewerSchema},
		{"corrupt json", `{"schema": 1, `, nil},
		{"wrong field type", `{"schema": 1, "series": {}}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, libraryFile)
			if err := os.WriteFile(path, []byte(tt.content), filePerm); err != nil {
				t.Fatal(err)
			}
			_, err := Open(context.Background(), dir, domain.FixedClock{T: t0}, Options{})
			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got, _ := os.ReadFile(path); string(got) != tt.content {
				t.Error("file was modified")
			}
		})
	}
}

func TestUpdate_BacksUpAtMostOncePerIntervalAndRotates(t *testing.T) {
	dir := t.TempDir()
	clock := &manualClock{t: t0}
	s, err := Open(context.Background(), dir, clock, Options{BackupsKeep: 2, BackupEvery: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	steps := []time.Duration{0, time.Minute, time.Hour, time.Minute, time.Hour, time.Hour}
	for i, d := range steps {
		clock.Advance(d)
		if err := s.Update(ctx, addSeries(string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	names, err := listBackups(filepath.Join(dir, backupsDir))
	if err != nil {
		t.Fatal(err)
	}
	// Backups at +1h01, +2h02 and +3h02 (the first update had no file yet);
	// only the newest 2 are kept.
	want := []string{"library-20261019-140200.json", "library-20261019-150200.json"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("backups = %v, want %v", names, want)
	}
	reopened, err := Open(ctx, dir, clock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.lastBackup.Equal(t0.Add(3*time.Hour + 2*time.Minute)) {
		t.Errorf("lastBackup = %v, want restored from newest backup name", reopened.lastBackup)
	}
}

func TestOpen_MigratesAfterBackingUpOriginal(t *testing.T) {
	dir := t.TempDir()
	copyFixture(t, "library-v1.json", dir)
	original, _ := os.ReadFile(filepath.Join(dir, libraryFile))
	addFlag := func(d map[string]json.RawMessage) error {
		d["migrated_flag"] = json.RawMessage(`true`)
		return nil
	}
	s, err := open(context.Background(), &Store{
		dir: dir, clock: domain.FixedClock{T: t0}, opts: Options{}.withDefaults(),
		schema: 2, steps: []migration{addFlag},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Library().Schema != 2 || s.Library().Extra["migrated_flag"] == nil {
		t.Errorf("library not migrated: schema=%d extra=%v", s.Library().Schema, s.Library().Extra)
	}
	backup, err := os.ReadFile(filepath.Join(dir, backupsDir, "library-20261019-120000.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Error("backup does not match the pre-migration file")
	}
	saved, _ := os.ReadFile(filepath.Join(dir, libraryFile))
	if !strings.Contains(string(saved), `"schema": 2`) || !strings.Contains(string(saved), `"sync_token": "abc"`) {
		t.Errorf("migrated file not written correctly:\n%s", saved)
	}
}

func TestBackup_ForcesACopy(t *testing.T) {
	dir := t.TempDir()
	clock := &manualClock{t: t0}
	s, err := Open(context.Background(), dir, clock, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(context.Background(), addSeries("a")); err != nil {
		t.Fatal(err)
	}
	clock.Advance(time.Minute) // well within the hourly limit
	if err := s.Backup(context.Background()); err != nil {
		t.Fatal(err)
	}
	names, _ := listBackups(filepath.Join(dir, backupsDir))
	if len(names) != 1 || names[0] != "library-20261019-120100.json" {
		t.Errorf("backups = %v", names)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Backup(canceled); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}
