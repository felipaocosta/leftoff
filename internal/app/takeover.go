package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/felipaocosta/leftoff/internal/storage"
)

// takeoverWait is how long the other app gets to quit at each step.
const takeoverWait = 10 * time.Second

// Takeover frees a data folder held by another LeftOff, when the
// user asks for it (the "library open elsewhere" message): it asks the
// other app to quit through its API, ends the process if it does not
// answer (an old version, a hung one), and waits until the folder is
// free. Writes are atomic, so ending a process never leaves a broken file.
func Takeover(ctx context.Context, dataDir string) error {
	if folderFree(dataDir) {
		return nil
	}
	h, err := storage.ReadHolder(dataDir)
	if err != nil {
		return fmt.Errorf("takeover: %w", err)
	}
	if h.PID == 0 { // an empty or unreadable note: ask the system
		h.PID = lockHolderPID(storage.LockPath(dataDir))
	}
	if h.URL != "" {
		askToQuit(ctx, h.URL)
		if waitFree(ctx, dataDir, takeoverWait) {
			return nil
		}
	}
	// The lock is still held, so h.PID is its holder: no other program.
	if h.PID > 0 && h.PID != os.Getpid() && !folderFree(dataDir) {
		endProcess(ctx, h.PID, dataDir)
	}
	if waitFree(ctx, dataDir, takeoverWait) {
		return nil
	}
	return fmt.Errorf("takeover (%s): %w", h, storage.ErrLocked)
}

// askToQuit asks the app at base to quit (POST /app/quit); best effort.
func askToQuit(ctx context.Context, base string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"api/v1/app/quit", nil)
	if err != nil {
		return
	}
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close() // status only: the lock tells whether it quit
	}
}

// endProcess interrupts pid (it shuts down like on Ctrl+C) and kills it
// if the folder is still held after takeoverWait.
func endProcess(ctx context.Context, pid int, dataDir string) {
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if p.Signal(os.Interrupt) == nil && waitFree(ctx, dataDir, takeoverWait) {
		return
	}
	_ = p.Kill() // Windows has no interrupt; a hung process ignores it
}

// folderFree reports whether nobody holds dataDir (taking and giving back
// the lock).
func folderFree(dataDir string) bool {
	l, err := storage.LockDir(dataDir, "check")
	if err != nil {
		return false
	}
	_ = l.Release() // a check only
	return true
}

func waitFree(ctx context.Context, dataDir string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if folderFree(dataDir) {
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}
