// Package testfixture reads the repository's testdata/ (saved site pages,
// golden results, libraries from older versions) for tests. That folder
// is not published with the code, so a test that needs it is skipped,
// not failed, where it is missing. It is only imported by tests.
package testfixture

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var (
	once    sync.Once
	present bool
)

// Require skips tb when the repository's testdata/ is absent.
func Require(tb testing.TB) {
	tb.Helper()
	once.Do(func() { present = found() })
	if !present {
		tb.Skip("testdata/ (saved site pages) is not in this checkout")
	}
}

// ReadFile is os.ReadFile for a file under testdata/, skipping tb when the
// folder is absent. A missing file in a present folder is still an error.
func ReadFile(tb testing.TB, path string) ([]byte, error) {
	tb.Helper()
	Require(tb)
	return os.ReadFile(path)
}

// found looks for testdata/ next to go.mod, walking up from the package.
func found() bool {
	dir, err := os.Getwd()
	if err != nil {
		return false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			info, err := os.Stat(filepath.Join(dir, "testdata"))
			return err == nil && info.IsDir()
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
