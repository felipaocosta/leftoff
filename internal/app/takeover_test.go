package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/storage"
)

// The holder answers: it is asked to quit through its API.
func TestTakeover_AsksTheOtherAppToQuit(t *testing.T) {
	dir := t.TempDir()
	done := make(chan error, 1)
	ready := make(chan string, 1)
	go func() {
		done <- Serve(context.Background(), Config{DataDir: dir, MinInterval: time.Millisecond}, discard(),
			ServeOptions{Host: "127.0.0.1", AppMode: true, Version: "old", OnReady: func(a string) { ready <- a }})
	}()
	<-ready
	if err := Takeover(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("the other app: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the other app did not quit")
	}
	if !folderFree(dir) {
		t.Error("the folder is still held")
	}
}

// The holder does not answer (no API): its process is ended, whatever its
// note says (this version's, the first version's one-liner, or nothing).
func TestTakeover_EndsAHolderThatDoesNotAnswer(t *testing.T) {
	if os.Getenv("MT_HOLD_DIR") != "" {
		return // the helper process runs TestHelperHoldLock instead
	}
	for _, note := range []string{"current", "legacy", "empty"} {
		t.Run(note, func(t *testing.T) { takeoverHelper(t, note) })
	}
}

func takeoverHelper(t *testing.T, note string) {
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHelperHoldLock$")
	cmd.Env = append(os.Environ(), "MT_HOLD_DIR="+dir, "MT_HOLD_NOTE="+note)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); close(exited) }()
	deadline := time.Now().Add(10 * time.Second)
	for _, err := os.Stat(filepath.Join(dir, "held")); err != nil; _, err = os.Stat(filepath.Join(dir, "held")) {
		if time.Now().After(deadline) {
			t.Fatal("the helper never took the lock")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := Takeover(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("the holder was not ended")
	}
}

// TestHelperHoldLock holds MT_HOLD_DIR and never answers (for the test above).
func TestHelperHoldLock(t *testing.T) {
	dir := os.Getenv("MT_HOLD_DIR")
	if dir == "" {
		t.Skip("helper process only")
	}
	if _, err := storage.LockDir(dir, "hung 0.1"); err != nil {
		t.Fatal(err)
	}
	switch os.Getenv("MT_HOLD_NOTE") {
	case "legacy": // the first version's note
		_ = os.WriteFile(storage.LockPath(dir), []byte(fmt.Sprintf("PID %d, app 8b326ae\n", os.Getpid())), 0o600)
	case "empty":
		_ = os.WriteFile(storage.LockPath(dir), nil, 0o600)
	}
	_ = os.WriteFile(filepath.Join(dir, "held"), nil, 0o600) // marks the helper as started
	time.Sleep(time.Minute)
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }
