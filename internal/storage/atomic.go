package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// staleTempAge is how old a temp file must be to be removed: younger ones
// may belong to a write still in progress (another process on DATA_DIR).
const staleTempAge = time.Hour

// removeStaleTemps deletes the temp files that writeFileAtomic leaves in
// dir when the process dies before the rename. They are never the current
// data (the rename did not happen), so removing them loses nothing.
func removeStaleTemps(dir string, now time.Time) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.*.tmp"))
	if err != nil {
		return // the pattern is constant and valid
	}
	for _, path := range matches {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || now.Sub(info.ModTime()) < staleTempAge {
			continue
		}
		_ = os.Remove(path) // best effort: a leftover only takes disk space
	}
}

// writeFileAtomic writes data to path so that readers see either the old or
// the new content, never a partial file: it writes a temp file in the same
// directory, fsyncs it, renames it over path and fsyncs the directory.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name()) // best effort; the original error matters more
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close() // already failing; Write's error is the one to report
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close() // already failing; Sync's error is the one to report
		return fmt.Errorf("fsync %s: %w", tmp.Name(), err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err = os.Chmod(tmp.Name(), perm); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	return syncDir(dir)
}

// syncDir fsyncs a directory so a rename inside it is durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open dir %s: %w", dir, err)
	}
	defer func() { _ = d.Close() }() // read-only handle; close errors are irrelevant
	if err := d.Sync(); err != nil {
		return fmt.Errorf("fsync dir %s: %w", dir, err)
	}
	return nil
}
