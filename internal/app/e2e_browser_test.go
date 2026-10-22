//go:build e2eweb

package app

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// TestServeForBrowser runs the real server for the Playwright tests
// (web/e2e): the built UI and the API on E2E_PORT, with the fake outside
// world of the end-to-end tests and an empty data dir. A control server on
// E2E_CONTROL_PORT lets a test publish a chapter (POST /release) or take
// it back (POST /reset). It runs
// until Playwright stops it:
//
//	E2E_PORT=18765 E2E_CONTROL_PORT=18766 go test -tags e2eweb -run TestServeForBrowser -timeout 0 ./internal/app
func TestServeForBrowser(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("E2E_PORT"))
	if err != nil {
		t.Skip("E2E_PORT not set: run through npm run e2e in web/")
	}
	w := newWorld()
	release := grandFinale(t, w)
	for slug, file := range map[string]string{
		"https://www.mangaread.org/manga/eleceed/":                              "mangaread.org/eleceed.html",
		"https://www.mangaread.org/manga/hunter-academys-strongest-battle-god/": "mangaread.org/hunter-academys-strongest-battle-god.html",
		"https://ravenscans.org/manga/the-extras-academy-survival-guide/":       "ravenscans.org/the-extras-academy-survival-guide.html",
		"https://vortexscans.org/series/survival-supremacy":                     "vortexscans.org/survival-supremacy.html",
		"https://roliascan.com/manga/healing-at-the-academy-convenience-store/": "roliascan.com/healing-at-the-academy-convenience-store.html",
	} {
		w.set(slug, fixture(t, file))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	control := &http.Server{Handler: http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/release":
			release()
		case r.Method == http.MethodPost && r.URL.Path == "/reset":
			release = grandFinale(t, w) // chapter 50 not out yet
		}
		rw.WriteHeader(http.StatusNoContent)
	}), ReadHeaderTimeout: time.Second}
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:"+os.Getenv("E2E_CONTROL_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = control.Serve(ln) }() // stopped below
	defer func() { _ = control.Close() }()

	cfg := Config{DataDir: t.TempDir(), Port: port, MinInterval: time.Millisecond, MangaDexLangs: []string{"pt-br", "en"}}
	if err := Serve(ctx, cfg, slog.New(slog.DiscardHandler), ServeOptions{Version: "e2e", Host: "127.0.0.1", Transport: w}); err != nil {
		t.Fatal(err)
	}
}
