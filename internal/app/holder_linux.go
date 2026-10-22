//go:build linux

package app

import (
	"os"
	"path/filepath"
	"strconv"
)

// lockHolderPID finds the process that has lockPath open (the data folder
// lock holder) by its open files; 0 when none is found.
func lockHolderPID(lockPath string) int {
	want, err := filepath.EvalSymlinks(lockPath)
	if err != nil {
		return 0
	}
	fds, err := filepath.Glob("/proc/[0-9]*/fd/*")
	if err != nil {
		return 0
	}
	for _, fd := range fds {
		if target, err := os.Readlink(fd); err == nil && target == want {
			pid, err := strconv.Atoi(filepath.Base(filepath.Dir(filepath.Dir(fd))))
			if err == nil && pid != os.Getpid() {
				return pid
			}
		}
	}
	return 0
}
