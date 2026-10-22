package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/app"
	"github.com/felipaocosta/leftoff/internal/storage"
	"github.com/felipaocosta/leftoff/internal/tray"
)

// freePort returns a TCP port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func post(t *testing.T, url string) int {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestRunApp_SingleInstanceOpenBrowserAndQuit(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	t.Setenv("PORT", strconv.Itoa(port))
	cfg := app.Config{DataDir: dir, Port: port}
	base := "http://127.0.0.1:" + strconv.Itoa(port) + "/"

	opened := make(chan string, 2)
	deps := appDeps{
		openURL: func(_ context.Context, u string) error { opened <- u; return nil },
		stderr:  &bytes.Buffer{},
	}
	done := make(chan error, 1)
	go func() { done <- runApp(context.Background(), cfg, deps) }()

	select {
	case u := <-opened:
		if u != base {
			t.Fatalf("opened %q, want %q", u, base)
		}
	case err := <-done:
		t.Fatalf("app exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("browser was not opened")
	}

	// A second launch finds the running instance and only opens the browser.
	if err := runApp(context.Background(), cfg, deps); err != nil {
		t.Fatal(err)
	}
	if u := <-opened; u != base {
		t.Errorf("second launch opened %q", u)
	}

	if code := post(t, base+"api/v1/app/quit"); code != http.StatusAccepted {
		t.Fatalf("quit: %d", code)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("app did not stop after quit")
	}
	log, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil || !strings.Contains(string(log), `"app_mode":true`) || !strings.Contains(string(log), "shutting down") {
		t.Errorf("app.log = %s (%v)", log, err)
	}
}

// Another LeftOff (on another port) holds the library: app mode
// shows a page in the browser; its button frees the folder and the app
// then starts for real.
func TestRunApp_LibraryOpenElsewhere(t *testing.T) {
	dir := t.TempDir()
	holder, err := storage.LockDir(dir, "desktop test")
	if err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	t.Setenv("PORT", strconv.Itoa(port))
	base := "http://127.0.0.1:" + strconv.Itoa(port) + "/"
	opened := make(chan string, 2)
	deps := appDeps{openURL: func(_ context.Context, u string) error { opened <- u; return nil }, stderr: &bytes.Buffer{}}
	done := make(chan error, 1)
	go func() { done <- runApp(context.Background(), app.Config{DataDir: dir, Port: port}, deps) }()

	page := <-opened
	if body := getBody(t, page); !strings.Contains(body, "desktop test") {
		t.Fatalf("locked page = %s", body)
	}
	_ = holder.Release() // the other app quits (Takeover is tested in internal/app)
	if code := post(t, page+"takeover"); code != http.StatusNoContent {
		t.Fatalf("takeover = %d", code)
	}
	if u := <-opened; u != base {
		t.Fatalf("then opened %q, want the app at %q", u, base)
	}
	if code := post(t, base+"api/v1/app/quit"); code != http.StatusAccepted {
		t.Fatalf("quit: %d", code)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }() // read below
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestInstanceRunning_FalseWhenNothingListens(t *testing.T) {
	if instanceRunning(context.Background(), "http://127.0.0.1:"+strconv.Itoa(freePort(t))+"/") {
		t.Error("nothing is listening")
	}
}

func TestHealthCommand(t *testing.T) {
	port := freePort(t)
	if err := healthCommand(context.Background(), app.Config{Port: port}); err == nil {
		t.Error("nothing listening: health must fail")
	}
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("PORT", strconv.Itoa(port))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, nil, &bytes.Buffer{}, &bytes.Buffer{}) }()
	deadline := time.Now().Add(5 * time.Second)
	for healthCommand(context.Background(), app.Config{Port: port}) != nil {
		if time.Now().After(deadline) {
			t.Fatal("server never became healthy")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRun_Commands(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	if err := run(context.Background(), []string{"version"}, &out, &errOut); err != nil || strings.TrimSpace(out.String()) != version {
		t.Errorf("version: %q %v", out.String(), err)
	}
	if err := run(context.Background(), []string{"extract"}, &out, &errOut); err == nil {
		t.Error("extract without url should fail")
	}
	if err := run(context.Background(), []string{"extract", "not-a-url"}, &out, &errOut); err == nil {
		t.Error("extract with invalid url should fail")
	}
	if err := run(context.Background(), []string{"dance"}, &out, &errOut); err == nil {
		t.Error("unknown command should fail")
	}
}

func TestServe_StartsAndShutsDown(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("PORT", "0")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	var out, errOut bytes.Buffer
	if err := run(ctx, nil, &out, &errOut); err != nil {
		t.Fatalf("serve: %v\n%s", err, errOut.String())
	}
	if !strings.Contains(errOut.String(), "server started") || !strings.Contains(errOut.String(), "shutting down") ||
		!strings.Contains(errOut.String(), `Z","level"`) {
		t.Errorf("logs = %s", errOut.String())
	}
}

// App mode has no window: its tray icon shows it runs, and Quit ends it.
func TestRunApp_TrayIconQuits(t *testing.T) {
	port := freePort(t)
	t.Setenv("PORT", strconv.Itoa(port))
	shown := make(chan tray.Options, 1)
	stopped := make(chan struct{})
	deps := appDeps{
		openURL: func(context.Context, string) error { return nil },
		stderr:  &bytes.Buffer{},
		tray: func(o tray.Options) func() {
			shown <- o
			return func() { close(stopped) }
		},
	}
	done := make(chan error, 1)
	go func() { done <- runApp(context.Background(), app.Config{DataDir: t.TempDir(), Port: port}, deps) }()
	var o tray.Options
	select {
	case o = <-shown:
	case <-time.After(5 * time.Second):
		t.Fatal("no tray icon")
	}
	if len(o.Items) != 3 || o.OnOpen == nil {
		t.Fatalf("tray = %+v", o)
	}
	o.Items[2].OnClick() // Quit
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Quit in the tray did not end the app")
	}
	<-stopped
}
