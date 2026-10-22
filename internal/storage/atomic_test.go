package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic_ReplacesContentAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	for _, content := range []string{"first", "second"} {
		if err := writeFileAtomic(path, []byte(content), filePerm); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("content = %q, want %q", got, content)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want only f.json", len(entries))
	}
}

func TestWriteFileAtomic_FailsWhenDirMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "f.json")
	if err := writeFileAtomic(path, []byte("x"), filePerm); err == nil {
		t.Error("expected error")
	}
}

func TestWriteFileAtomic_RenameFailureRemovesTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, dirPerm); err != nil { // renaming a file over a non-empty dir fails
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "x"), nil, filePerm); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(target, []byte("x"), filePerm); err == nil {
		t.Fatal("expected rename error")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}
