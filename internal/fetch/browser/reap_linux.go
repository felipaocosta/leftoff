//go:build rod && linux

package browser

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// reapProfile ends the processes still running with our browser profile
// (--user-data-dir=profile). A browser started through `flatpak run` lives
// on inside the sandbox after the launcher is killed: left alone, every
// solve could leave a headless browser behind (seen running for a day).
// The profile is the app's own, so the user's browser is never touched.
func reapProfile(profile string) {
	if profile == "" {
		return
	}
	marker := []byte("--user-data-dir=" + profile + "\x00")
	pids := profileProcesses(marker)
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGTERM) // already gone is fine
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(pids) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		pids = profileProcesses(marker)
	}
	for _, pid := range pids {
		_ = syscall.Kill(pid, syscall.SIGKILL) // it ignored SIGTERM
	}
}

// profileProcesses lists the processes whose command line has marker.
func profileProcesses(marker []byte) []int {
	dirs, err := filepath.Glob("/proc/[0-9]*")
	if err != nil {
		return nil
	}
	self := os.Getpid()
	var pids []int
	for _, d := range dirs {
		pid, err := strconv.Atoi(filepath.Base(d))
		if err != nil || pid == self {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join(d, "cmdline"))
		if err == nil && bytes.Contains(append(cmdline, 0), marker) {
			pids = append(pids, pid)
		}
	}
	return pids
}
