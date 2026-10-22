package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

// openBrowser opens url in the user's default browser. The launcher is
// detached from ctx so quitting the app never kills the browser.
func openBrowser(ctx context.Context, url string) error {
	ctx = context.WithoutCancel(ctx)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", url)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	go func() { _ = cmd.Wait() }() // reap the launcher; its exit status does not matter
	return nil
}
