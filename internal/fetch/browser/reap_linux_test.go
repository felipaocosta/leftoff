//go:build rod && linux

package browser

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestReapProfile_EndsOnlyProcessesWithOurProfile(t *testing.T) {
	profile := t.TempDir()
	loop := "while :; do sleep 0.05; done"
	ours := exec.CommandContext(t.Context(), "sh", "-c", loop, "sh", "--user-data-dir="+profile)
	other := exec.CommandContext(t.Context(), "sh", "-c", loop, "sh", "--user-data-dir="+profile+"-someone-else")
	for _, c := range []*exec.Cmd{ours, other} {
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { _ = other.Process.Kill(); _ = other.Wait() }() // test cleanup
	reapProfile(profile)
	done := make(chan error, 1)
	go func() { done <- ours.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the process with our profile is still running")
	}
	if err := syscall.Kill(other.Process.Pid, 0); err != nil {
		t.Errorf("a process with another profile was ended: %v", err)
	}
}
