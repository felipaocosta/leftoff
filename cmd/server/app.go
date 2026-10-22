package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/felipaocosta/leftoff/internal/app"
	"github.com/felipaocosta/leftoff/internal/fetch/browser"
	"github.com/felipaocosta/leftoff/internal/storage"
	"github.com/felipaocosta/leftoff/internal/tray"
)

// appPort is the fixed local port of the desktop app, so a second launch
// finds the running instance and bookmarks keep working.
const appPort = 8730

// appDeps are the side effects of the app command, replaceable in tests.
type appDeps struct {
	openURL func(ctx context.Context, url string) error
	stderr  io.Writer
	// tray shows the tray icon and returns its stop; nil shows none (tests).
	tray func(tray.Options) (stop func())
}

// runApp is the desktop mode: if an instance is already running it only
// opens the browser; otherwise it starts the server on 127.0.0.1, logs to
// <DATA_DIR>/app.log (a GUI build has no console) and opens the browser.
func runApp(ctx context.Context, cfg app.Config, deps appDeps) error {
	if os.Getenv("PORT") == "" {
		cfg.Port = appPort
	}
	if cfg.BrowserSolver == "" && browser.Available() {
		cfg.BrowserSolver = "interactive" // a desktop has a browser and a user to pass challenges
	}
	if cfg.FlareSolverrURL == "" {
		cfg.FlareSolverrURL = app.DetectFlareSolverr(ctx, app.LocalFlareSolverr) // tried before the browser when running
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", cfg.Port)
	if cfg.Port != 0 && instanceRunning(ctx, url) {
		return deps.openURL(ctx, url)
	}

	logger, closeLog, err := appLogger(cfg, deps.stderr)
	if err != nil {
		return err
	}
	defer closeLog()
	ctx, quit := context.WithCancel(ctx)
	defer quit()
	stopTray := func() {}
	defer func() { stopTray() }()
	err = app.Serve(ctx, cfg, logger, app.ServeOptions{
		Version: version,
		Host:    "127.0.0.1",
		AppMode: true,
		OnReady: func(addr string) {
			base := "http://" + addr + "/"
			if err := deps.openURL(ctx, base); err != nil {
				logger.Warn("open browser", "err", err)
			}
			if deps.tray != nil {
				stopTray = deps.tray(appTray(ctx, base, deps, quit))
			}
		},
	})
	if !errors.Is(err, storage.ErrLocked) {
		return err
	}
	// A GUI build has no console: ask in the browser, with a button that
	// closes the other app; then start for real.
	logger.Error("library open elsewhere", "err", err)
	pageCtx, cancel := context.WithTimeout(ctx, lockedPageWait)
	defer cancel()
	freed, perr := app.ServeLockedPage(pageCtx, app.SystemLanguage(os.Getenv), err.Error(), cfg.DataDir,
		func(u string) error { return deps.openURL(ctx, u) })
	if perr != nil || !freed {
		return err
	}
	closeLog()
	return runApp(ctx, cfg, deps)
}

// appTrayTexts are the tray menu labels of app mode.
var appTrayTexts = map[string]map[string]string{
	"pt-BR": {"open": "Abrir no navegador", "refresh": "Atualizar tudo", "quit": "Sair"},
	"en":    {"open": "Open in the browser", "refresh": "Refresh all", "quit": "Quit"},
}

// appTray is app mode's tray icon: app mode has no window, so the icon
// shows it is running and how to reach it or quit it.
func appTray(ctx context.Context, base string, deps appDeps, quit func()) tray.Options {
	t := appTrayTexts[app.SystemLanguage(os.Getenv)]
	open := func() { _ = deps.openURL(ctx, base) } // best effort: a click again retries
	return tray.Options{
		Tooltip: "LeftOff",
		OnOpen:  open,
		Items: []tray.Item{
			{Label: t["open"], OnClick: open},
			{Label: t["refresh"], OnClick: func() { postLocal(ctx, base+"api/v1/refresh") }},
			{Label: t["quit"], OnClick: quit},
		},
	}
}

// postLocal sends an empty POST to the local server; best effort.
func postLocal(ctx context.Context, url string) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return
	}
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close() // the job runs on its own
	}
}

// systemTray shows the icon when the system has a tray.
func systemTray(o tray.Options) func() {
	if !tray.Available() {
		return func() {}
	}
	return tray.Start(o)
}

// lockedPageWait is how long the "library open elsewhere" page waits for
// the user before this app gives up.
const lockedPageWait = 30 * time.Minute

// instanceRunning reports whether a LeftOff app already answers at url.
func instanceRunning(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"api/v1/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }() // read-only probe
	var h struct{ Status, Mode string }
	return json.NewDecoder(resp.Body).Decode(&h) == nil && h.Status == "ok" && h.Mode == "app"
}

// appLogger writes JSON logs to <DATA_DIR>/app.log (replaced on each start)
// and to stderr when there is one.
func appLogger(cfg app.Config, stderr io.Writer) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create data dir: %w", err)
	}
	f, err := os.Create(filepath.Join(cfg.DataDir, "app.log"))
	if err != nil {
		return nil, nil, fmt.Errorf("open log: %w", err)
	}
	out := io.MultiWriter(f, stderr)
	logger := slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: cfg.LogLevel, ReplaceAttr: utcTime}))
	logger.Info("app starting", "version", version, "port", strconv.Itoa(cfg.Port))
	return logger, func() { _ = f.Close() }, nil // closing a log file at exit; nothing to recover
}
