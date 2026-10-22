// Package main is the LeftOff server binary.
//
// It wires dependencies together (storage, fetch, extract, library, api)
// and runs one of its commands. It holds no business logic of its own.
//
//	server                 run the HTTP server (default)
//	server app             desktop mode: local server + opens the browser
//	server extract <url>   print the extraction result of a URL as JSON
//	server smoke [dir]     check the reference sites on the live web (dir: golden files, default testdata/pipeline)
//	server health          exit 0 if the local server answers (container healthcheck)
//	server version         print the version
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/felipaocosta/leftoff/internal/app"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

// defaultCommand runs when no command is given. Desktop packages (AppImage,
// Windows .exe) are built with -ldflags "-X main.defaultCommand=app".
var defaultCommand = "serve"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	cfg, err := app.LoadConfig(os.Getenv)
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: cfg.LogLevel, ReplaceAttr: utcTime}))
	cmd := defaultCommand
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		return app.Serve(ctx, cfg, logger, app.ServeOptions{Version: version})
	case "app":
		return runApp(ctx, cfg, appDeps{openURL: openBrowser, stderr: stderr, tray: systemTray})
	case "extract":
		if len(args) != 2 {
			return fmt.Errorf("usage: server extract <url>")
		}
		return app.ExtractCommand(ctx, cfg, logger, args[1], stdout)
	case "smoke":
		dir := "testdata/pipeline"
		if len(args) > 1 {
			dir = args[1]
		}
		return app.SmokeCommand(ctx, cfg, logger, dir, stdout)
	case "health":
		return healthCommand(ctx, cfg)
	case "version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	default:
		return fmt.Errorf("unknown command %q (want serve, app, extract, smoke, health or version)", cmd)
	}
}

// utcTime logs timestamps in UTC (RULES §4).
func utcTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
		a.Value = slog.TimeValue(a.Value.Time().UTC())
	}
	return a
}
