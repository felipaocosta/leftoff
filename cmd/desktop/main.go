// Command desktop is the LeftOff desktop app (Phase 6): a Wails
// window over the same server the other builds run (internal/app), on
// 127.0.0.1 and a fixed port (8732; a free one if taken). In connected mode the window opens a
// server's UI instead (settings.desktop); the "LeftOff" menu always
// leads back to the local library.
//
// Build with the tags desktop,production (plus webkit2_41 on Linux and rod
// for the Cloudflare browser solver): see deploy/desktop/build.sh.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/felipaocosta/leftoff/internal/app"
	"github.com/felipaocosta/leftoff/internal/fetch/browser"
	"github.com/felipaocosta/leftoff/internal/storage"
)

var version = "dev"

//go:embed icon.png
var icon []byte

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// desktopPort is the fixed local port of the desktop app (the AppImage
// and portable app mode use 8730, the Android core 8731).
const desktopPort = 8732

func run(ctx context.Context) error {
	cfg, err := app.LoadConfig(os.Getenv)
	if err != nil {
		return err
	}
	if os.Getenv("PORT") == "" {
		// A fixed port keeps the page's origin, and the preferences the
		// window keeps for it (theme, language), across launches; a taken
		// port falls back to a free one (ServeOptions.AnyPortIfBusy).
		cfg.Port = desktopPort
	}
	if cfg.BrowserSolver == "" && browser.Available() {
		cfg.BrowserSolver = "interactive" // a desktop has a browser and a user to pass challenges
	}
	if cfg.FlareSolverrURL == "" {
		cfg.FlareSolverrURL = app.DetectFlareSolverr(ctx, app.LocalFlareSolverr)
	}
	logger, closeLog, err := desktopLogger(cfg)
	if err != nil {
		return err
	}
	defer closeLog()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := newWindow(logger)
	ready := make(chan string, 1)
	served := make(chan error, 1)
	go func() { // owned by ctx: Serve returns when it is canceled or the UI quits
		served <- app.Serve(ctx, cfg, logger, app.ServeOptions{
			Version: version, Host: "127.0.0.1", Desktop: true, SaveFile: w.saveFile, AnyPortIfBusy: true,
			OnShow:  w.show,
			OnReady: func(addr string) { ready <- addr },
		})
	}()
	select {
	case addr := <-ready:
		w.local = "http://" + addr + "/"
	case err := <-served:
		if errors.Is(err, storage.ErrLocked) {
			if showRunning(ctx, cfg.DataDir) {
				return nil // the desktop app is already running (maybe in the tray): its window is back
			}
			logger.Error("library open elsewhere", "err", err)
			return w.showLocked(menuLanguage(), err.Error(), cfg.DataDir)
		}
		return fmt.Errorf("start server: %w", err)
	}
	if err := w.run(ctx, served); err != nil {
		return err
	}
	cancel()
	return nil
}

// desktopLogger writes to <DATA_DIR>/desktop.log and stderr.
func desktopLogger(cfg app.Config) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create data dir: %w", err)
	}
	f, err := os.Create(filepath.Join(cfg.DataDir, "desktop.log"))
	if err != nil {
		return nil, nil, fmt.Errorf("open log: %w", err)
	}
	logger := slog.New(slog.NewJSONHandler(io.MultiWriter(f, os.Stderr), &slog.HandlerOptions{Level: cfg.LogLevel}))
	logger.Info("desktop starting", "version", version)
	return logger, func() { _ = f.Close() }, nil // closing the log at exit; nothing to recover
}

// showRunning asks the desktop app that holds dataDir to show its window,
// as a second launch of a tray app does; false when the holder is another
// kind of app or does not answer (then the locked message is shown).
func showRunning(ctx context.Context, dataDir string) bool {
	h, err := storage.ReadHolder(dataDir)
	if err != nil || !strings.HasPrefix(h.Owner, "desktop ") || h.URL == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL+"api/v1/app/show", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close() // status only
	return resp.StatusCode == http.StatusAccepted
}
