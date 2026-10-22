package main

import (
	"context"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/felipaocosta/leftoff/internal/tray"
)

// trayTexts are the tray menu labels.
var trayTexts = map[string]map[string]string{
	"pt-BR": {"open": "Abrir LeftOff", "refresh": "Atualizar tudo", "quit": "Sair"},
	"en":    {"open": "Open LeftOff", "refresh": "Refresh all", "quit": "Quit"},
}

// startTray shows the icon in the system tray, like Steam: the app keeps
// checking for new chapters with its window closed, and the icon shows it
// is still running. Call it once Wails has started (w.ctx is set).
func (w *window) startTray(lang string) (stop func()) {
	t := trayTexts[lang]
	return tray.Start(tray.Options{
		Tooltip: "LeftOff",
		OnOpen:  w.show,
		Items: []tray.Item{
			{Label: t["open"], OnClick: w.show},
			{Label: t["refresh"], OnClick: w.refreshAll},
			{Label: t["quit"], OnClick: func() {
				if ctx := w.wails(); ctx != nil {
					runtime.Quit(ctx)
				}
			}},
		},
	})
}

// show brings the window back (from the tray, or a second launch).
func (w *window) show() {
	ctx := w.wails()
	if ctx == nil {
		return // not started yet: it is about to show anyway
	}
	runtime.WindowShow(ctx)
	runtime.WindowUnminimise(ctx)
}

// refreshAll starts "refresh all" on the local server (RF-21).
func (w *window) refreshAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.local+"api/v1/refresh", nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.log.Warn("tray refresh all", "err", err)
		return
	}
	_ = resp.Body.Close() // the job runs on its own
}
