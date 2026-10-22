package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

const (
	libraryFile = "library.json"
	backupsDir  = "backups"
	dirPerm     = 0o755
	filePerm    = 0o644
)

// Options tunes a Store. Zero values select the defaults.
type Options struct {
	BackupsKeep int           // backups to keep; default 10 (settings.backups_keep)
	BackupEvery time.Duration // minimum interval between usage backups; default 1h
}

func (o Options) withDefaults() Options {
	if o.BackupsKeep <= 0 {
		o.BackupsKeep = 10
	}
	if o.BackupEvery <= 0 {
		o.BackupEvery = time.Hour
	}
	return o
}

// Store owns library.json inside a data directory. It is safe for
// concurrent use: there is a single writer at a time and readers get an
// immutable snapshot.
type Store struct {
	dir    string
	clock  domain.Clock
	opts   Options
	schema int         // schema written by Update; CurrentSchema outside tests
	steps  []migration // migrations applied on open

	mu         sync.Mutex // serializes Update; guards lastBackup
	lastBackup time.Time
	snap       atomic.Pointer[Library]

	settingsMu sync.Mutex // serializes SaveSettings
	settings   atomic.Pointer[Settings]
}

// Open loads (or initializes) the library in dir, migrating it to
// CurrentSchema if needed. A backup is taken before any migration.
func Open(ctx context.Context, dir string, clock domain.Clock, opts Options) (*Store, error) {
	return open(ctx, &Store{
		dir: dir, clock: clock, opts: opts.withDefaults(),
		schema: CurrentSchema, steps: migrations(),
	})
}

func open(ctx context.Context, s *Store) (*Store, error) {
	for _, d := range []string{s.backupDir(), filepath.Join(s.dir, coversDir)} {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			return nil, fmt.Errorf("create data dir %s: %w", d, err)
		}
	}
	now := s.clock.Now()
	for _, d := range []string{s.dir, s.backupDir(), filepath.Join(s.dir, coversDir), filepath.Join(s.dir, recipesDir)} {
		removeStaleTemps(d, now)
	}
	last, err := latestBackupTime(s.backupDir())
	if err != nil {
		return nil, err
	}
	s.lastBackup = last

	settings, err := s.loadSettings()
	if err != nil {
		return nil, err
	}
	s.settings.Store(settings)

	lib, migrated, err := s.load()
	if err != nil {
		return nil, err
	}
	s.snap.Store(lib)
	debug.FreeOSMemory()
	if !migrated {
		return s, nil
	}
	if err := s.backupNow(); err != nil {
		return nil, fmt.Errorf("backup before migration: %w", err)
	}
	if err := s.Update(ctx, func(*Library) error { return nil }); err != nil {
		return nil, fmt.Errorf("write migrated library: %w", err)
	}
	return s, nil
}

// Library returns the current snapshot. It must not be modified.
func (s *Store) Library() *Library { return s.snap.Load() }

// Update applies fn to a copy of the library and, if fn succeeds, writes it
// atomically and publishes it as the new snapshot. If fn or the write fails,
// nothing changes. Derived fields and updated_at are refreshed on every write.
func (s *Store) Update(ctx context.Context, fn func(*Library) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update library: %w", err)
	}
	next, err := s.snap.Load().clone()
	if err != nil {
		return err
	}
	if err := fn(next); err != nil {
		return err
	}
	for i := range next.Series {
		next.Series[i].RecomputeDerived()
	}
	next.Schema = s.schema
	next.UpdatedAt = s.clock.Now()

	data, err := encodeLibrary(next)
	if err != nil {
		return err
	}
	if next.UpdatedAt.Sub(s.lastBackup) >= s.opts.BackupEvery {
		if err := s.backupNow(); err != nil {
			return err
		}
	}
	if err := writeFileAtomic(s.libraryPath(), data, filePerm); err != nil {
		return err
	}
	s.snap.Store(next)
	debug.FreeOSMemory()
	return nil
}

// Backup copies the current library file into backups/ right away (for
// example before an import), regardless of the usual hourly limit.
func (s *Store) Backup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backupNow()
}

// load reads library.json; a missing file yields an empty library.
func (s *Store) load() (lib *Library, migrated bool, err error) {
	data, err := os.ReadFile(s.libraryPath())
	if errors.Is(err, os.ErrNotExist) {
		lib := newLibrary()
		lib.Schema = s.schema
		return lib, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", s.libraryPath(), err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", s.libraryPath(), err)
	}
	migrated, err = migrate(doc, s.schema, s.steps)
	if err != nil {
		return nil, false, fmt.Errorf("migrate %s: %w", s.libraryPath(), err)
	}
	if migrated {
		if data, err = json.Marshal(doc); err != nil {
			return nil, false, fmt.Errorf("re-encode migrated library: %w", err)
		}
	}
	lib = &Library{}
	if err := json.Unmarshal(data, lib); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", s.libraryPath(), err)
	}
	if lib.Series == nil {
		lib.Series = []domain.Series{}
	}
	return lib, migrated, nil
}

// backupNow copies the on-disk library into backups/. Callers hold s.mu or
// are the only goroutine with access to s (Open).
func (s *Store) backupNow() error {
	now := s.clock.Now()
	if err := backupFile(s.libraryPath(), s.backupDir(), now, s.opts.BackupsKeep); err != nil {
		return err
	}
	s.lastBackup = now
	return nil
}

func (s *Store) libraryPath() string { return filepath.Join(s.dir, libraryFile) }
func (s *Store) backupDir() string   { return filepath.Join(s.dir, backupsDir) }
