package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	backupPrefix = "library-"
	backupSuffix = ".json"
	backupLayout = "20060102-150405"
)

// backupFile copies the current library file (if any) into the backups
// directory as library-YYYYMMDD-HHMMSS.json and prunes old backups,
// keeping the newest keep files.
func backupFile(libPath, backupDir string, now time.Time, keep int) error {
	data, err := os.ReadFile(libPath)
	if os.IsNotExist(err) {
		return nil // nothing to back up yet
	}
	if err != nil {
		return fmt.Errorf("read library for backup: %w", err)
	}
	name := backupPrefix + now.UTC().Format(backupLayout) + backupSuffix
	if err := writeFileAtomic(filepath.Join(backupDir, name), data, filePerm); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return pruneBackups(backupDir, keep)
}

// listBackups returns backup file names sorted from oldest to newest
// (the timestamp layout sorts lexicographically).
func listBackups(backupDir string) ([]string, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasPrefix(n, backupPrefix) && strings.HasSuffix(n, backupSuffix) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}

func pruneBackups(backupDir string, keep int) error {
	names, err := listBackups(backupDir)
	if err != nil {
		return err
	}
	for len(names) > keep {
		if err := os.Remove(filepath.Join(backupDir, names[0])); err != nil {
			return fmt.Errorf("prune backup %s: %w", names[0], err)
		}
		names = names[1:]
	}
	return nil
}

// latestBackupTime returns the timestamp of the newest backup, or the zero
// time when there is none or its name cannot be parsed.
func latestBackupTime(backupDir string) (time.Time, error) {
	names, err := listBackups(backupDir)
	if err != nil || len(names) == 0 {
		return time.Time{}, err
	}
	stamp := strings.TrimSuffix(strings.TrimPrefix(names[len(names)-1], backupPrefix), backupSuffix)
	t, err := time.Parse(backupLayout, stamp)
	if err != nil {
		return time.Time{}, nil //nolint:nilerr // a foreign file name just means "no usable backup"
	}
	return t, nil
}
