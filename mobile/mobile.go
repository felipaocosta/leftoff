// Package mobile is the Go core of the Android app (Phase 7), exported
// with gomobile bind: Start runs the same server as the other builds
// (internal/app) on 127.0.0.1:8731 (a free port if taken), and TargetURL says what
// the WebView should open (the local library or, in connected mode, a
// server's UI).
//
// gomobile only exports simple types, so errors are returned as Go errors
// (Java exceptions) and URLs as strings. It does NOT draw anything: the
// Kotlin shell in android/ owns the WebView.
package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/felipaocosta/leftoff/internal/app"
	"github.com/felipaocosta/leftoff/internal/fetch/browser"
	"github.com/felipaocosta/leftoff/internal/scheduler"
)

var (
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan error
	baseURL string
	sched   *scheduler.Scheduler
	tab     browser.Driver // the WebView lent by SetBrowser
	// The server runs while the app (Start/Stop) or a background check
	// (CheckNow) holds it, so a check never stops the open app's server.
	appHold    bool
	checkHolds int
)

// androidPort is the fixed local port: the WebView keeps the preferences
// (theme, language, filters) per origin, so a new port at every launch
// would forget them. A taken port falls back to a free one.
const androidPort = 8731

// Start runs the server with its data in dataDir (the app's internal
// storage) and returns its URL, e.g. "http://127.0.0.1:43123/". Calling it
// again while running returns the same URL.
func Start(dataDir string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	u, err := startLocked(dataDir)
	if err == nil {
		appHold = true
	}
	return u, err
}

// startLocked starts the server unless it is running; mu is held.
func startLocked(dataDir string) (string, error) {
	if baseURL != "" {
		return baseURL, nil
	}
	env := map[string]string{"DATA_DIR": dataDir, "PORT": strconv.Itoa(androidPort), "HOME": dataDir}
	cfg, err := app.LoadConfig(func(k string) string {
		if v, ok := env[k]; ok {
			return v
		}
		return os.Getenv(k)
	})
	if err != nil {
		return "", err
	}
	logger, err := fileLogger(dataDir)
	if err != nil {
		return "", err
	}
	ctx, stop := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	served := make(chan error, 1)
	var s *scheduler.Scheduler // set before ready is sent
	go func() {                // owned by cancel (Stop) or the process
		served <- app.Serve(ctx, cfg, logger, app.ServeOptions{
			Version: Version, Host: "127.0.0.1", Desktop: true, Mode: "android", AnyPortIfBusy: true, Browser: tab,
			OnScheduler: func(sc *scheduler.Scheduler) { s = sc },
			OnReady:     func(addr string) { ready <- addr },
		})
	}()
	select {
	case addr := <-ready:
		cancel, done, baseURL, sched = stop, served, "http://"+addr+"/", s
		return baseURL, nil
	case err := <-served:
		stop()
		return "", fmt.Errorf("start server: %w", err)
	case <-time.After(30 * time.Second):
		stop()
		return "", errors.New("start server: timed out")
	}
}

// Version is the app version, set by the build (-ldflags -X).
var Version = "dev"

// TargetURL is where the WebView should go: the local library, or the
// server of settings.desktop in connected mode. Start must have run.
func TargetURL() string {
	mu.Lock()
	local := baseURL
	mu.Unlock()
	if local == "" {
		return ""
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, local+"api/v1/settings", nil)
	if err != nil {
		return local
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return local
	}
	defer func() { _ = resp.Body.Close() }() // read-only
	var v struct {
		Desktop *struct {
			Mode      string `json:"mode"`
			ServerURL string `json:"server_url"`
		} `json:"desktop"`
	}
	if json.NewDecoder(resp.Body).Decode(&v) != nil || v.Desktop == nil ||
		v.Desktop.Mode != "connected" || v.Desktop.ServerURL == "" {
		return local
	}
	return v.Desktop.ServerURL + "/"
}

// LocalURL is the local server's URL ("" before Start).
func LocalURL() string {
	mu.Lock()
	defer mu.Unlock()
	return baseURL
}

// Stop releases the app's hold: the server shuts down (and Stop waits for
// it) unless a background check is still running.
func Stop() {
	mu.Lock()
	appHold = false
	stopLocked()
}

// stopLocked shuts the server down if nothing holds it; it unlocks mu.
func stopLocked() {
	if appHold || checkHolds > 0 {
		mu.Unlock()
		return
	}
	c, d := cancel, done
	cancel, done, baseURL, sched = nil, nil, "", nil
	mu.Unlock()
	if c != nil {
		c()
		<-d
	}
}

func fileLogger(dataDir string) (*slog.Logger, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dataDir, "android.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	// The file stays open for the process lifetime, like stderr.
	return slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})), nil
}
