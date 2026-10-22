package storage

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLockDir_OneHolderAtATime(t *testing.T) {
	dir := t.TempDir()
	first, err := LockDir(dir, "desktop 1.0")
	if err != nil {
		t.Fatal(err)
	}
	// A second holder (another process, or this one again) is refused and
	// told who has the folder.
	if _, err := LockDir(dir, "app 0.9"); !errors.Is(err, ErrLocked) || !strings.Contains(err.Error(), "desktop 1.0") {
		t.Fatalf("second lock: %v", err)
	}
	first.SetURL("http://127.0.0.1:8732/")
	if h, err := ReadHolder(dir); err != nil || h.Owner != "desktop 1.0" || h.URL != "http://127.0.0.1:8732/" || h.PID == 0 {
		t.Errorf("holder = %+v, %v", h, err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	again, err := LockDir(dir, "app 0.9")
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	_ = again.Release() // test cleanup
	// A holder of the first version wrote a single line.
	if err := os.WriteFile(filepath.Join(dir, lockFile), []byte("PID 1546125, app 8b326ae\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if h, err := ReadHolder(dir); err != nil || h.PID != 1546125 || h.Owner != "app 8b326ae" || h.URL != "" {
		t.Errorf("legacy holder = %+v, %v", h, err)
	}
	if err := (*DirLock)(nil).Release(); err != nil {
		t.Error(err)
	}
}
