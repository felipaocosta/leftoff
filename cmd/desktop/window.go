package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/felipaocosta/leftoff/internal/api"
	"github.com/felipaocosta/leftoff/internal/app"
	"github.com/felipaocosta/leftoff/internal/tray"
)

// window is the Wails window and where it points.
type window struct {
	local string // the local server, http://127.0.0.1:<port>/
	log   *slog.Logger
	ctx   context.Context // Wails' context, set on startup
	mu    sync.Mutex      // guards ctx: the tray and the API read it
}

// wails is Wails' context, or nil before startup.
func (w *window) wails() context.Context {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.ctx
}

// newWindow returns the window; local is set once the server listens.
func newWindow(logger *slog.Logger) *window {
	return &window{log: logger}
}

// saveFile implements app.ServeOptions.SaveFile with the native "save as"
// dialog: the WebView cannot download the library export.
func (w *window) saveFile(_ context.Context, name string, data []byte) (string, error) {
	if w.wails() == nil {
		return "", errors.New("the window is not open")
	}
	dir, _ := os.UserHomeDir() // "" lets the dialog pick its default
	path, err := runtime.SaveFileDialog(w.wails(), runtime.SaveDialogOptions{
		DefaultDirectory: dir, DefaultFilename: name, Title: "LeftOff",
		Filters: []runtime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if path == "" {
		return "", api.ErrSaveCanceled
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("save export: %w", err)
	}
	return path, nil
}

// run opens the window until it is closed, the UI quits (served returns)
// or ctx is canceled.
// showLocked opens a small window saying the library is open in another
// LeftOff, with a button that closes that one (app.Takeover) and
// starts this app again.
func (w *window) showLocked(lang, detail, dataDir string) error {
	var wctx context.Context
	err := wails.Run(&options.App{
		Title: "LeftOff", Width: 640, Height: 400,
		AssetServer: &assetserver.Options{Handler: app.LockedPage(lang, detail, dataDir, func() {
			if err := relaunch(); err != nil {
				w.log.Error("start again", "err", err)
				return
			}
			runtime.Quit(wctx)
		})},
		OnStartup: func(ctx context.Context) { wctx = ctx },
	})
	if err != nil {
		return fmt.Errorf("locked window: %w", err)
	}
	return nil
}

// relaunch starts this executable again with the same arguments.
func relaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("relaunch: %w", err)
	}
	cmd := exec.CommandContext(context.Background(), exe, os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("relaunch: %w", err)
	}
	return cmd.Process.Release()
}

func (w *window) run(ctx context.Context, served <-chan error) error {
	desk := w.desktopSettings(ctx)
	lang := menuLanguage()
	// With a tray, closing the window hides it (the icon brings it back);
	// without one it quits, or the app would run on unseen.
	hideOnClose := tray.Available()
	stopTray := func() {}
	err := wails.Run(&options.App{
		HideWindowOnClose: hideOnClose,
		Title:             "LeftOff",
		Width:             1200,
		Height:            820,
		MinWidth:          360,
		MinHeight:         480,
		AssetServer: &assetserver.Options{
			Handler: startPage(w.local, desk, lang),
		},
		Menu: w.menu(lang, desk.ServerURL),
		OnStartup: func(wctx context.Context) {
			w.mu.Lock()
			w.ctx = wctx
			w.mu.Unlock()
			if hideOnClose {
				stopTray = w.startTray(lang)
			}
			go func() { // owned by the window: ends with the server or the app
				select {
				case err := <-served:
					if err != nil {
						w.log.Error("server stopped", "err", err)
					}
				case <-ctx.Done():
				}
				runtime.Quit(wctx)
			}()
		},
		OnShutdown: func(context.Context) { stopTray() },
		Linux:      &linux.Options{Icon: icon, ProgramName: "leftoff", WebviewGpuPolicy: linux.WebviewGpuPolicyOnDemand},
	})
	if err != nil {
		return fmt.Errorf("window: %w", err)
	}
	return nil
}

// desktopSettings reads settings.desktop from the local API; standalone
// when it cannot.
func (w *window) desktopSettings(ctx context.Context) desktopSettings {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var v struct {
		Desktop *desktopSettings `json:"desktop"`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.local+"api/v1/settings", nil)
	if err == nil {
		var resp *http.Response
		if resp, err = http.DefaultClient.Do(req); err == nil {
			defer func() { _ = resp.Body.Close() }() // read-only
			err = json.NewDecoder(resp.Body).Decode(&v)
		}
	}
	if err != nil || v.Desktop == nil {
		w.log.Warn("read desktop settings; opening the local library", "err", err)
		return desktopSettings{Mode: "standalone"}
	}
	return *v.Desktop
}

type desktopSettings struct {
	Mode      string `json:"mode"`
	ServerURL string `json:"server_url"`
}

// navigate points the window at url.
func (w *window) navigate(url string) {
	if w.wails() == nil {
		return
	}
	data, _ := json.Marshal(url) // a string always encodes
	runtime.WindowExecJS(w.wails(), "window.location.assign("+string(data)+")")
}
