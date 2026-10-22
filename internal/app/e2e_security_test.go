package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/felipaocosta/leftoff/internal/storage"
)

func TestE2E_AuthToken(t *testing.T) {
	w := newWorld()
	grandFinale(t, w)
	srv := startServer(t, Config{DataDir: t.TempDir(), AuthToken: "s3cret-token"}, w, ServeOptions{})
	var s apiSeries
	if code := srv.call("POST", "/series", map[string]any{"url": gfURL}, &s); code != 201 {
		t.Fatalf("add with the token = %d", code)
	}
	anon := *srv
	anon.token = ""
	var e apiError
	for _, path := range []string{"/series", "/series/" + s.ID, "/export", "/settings"} {
		if code := anon.call("GET", path, nil, &e); code != 401 || e.Error.Code != "unauthorized" {
			t.Errorf("GET %s without token = %d %s", path, code, e.Error.Code)
		}
	}
	wrong := anon
	wrong.token = "guess"
	if code := wrong.call("DELETE", "/series/"+s.ID, nil, &e); code != 401 {
		t.Errorf("DELETE with a wrong token = %d", code)
	}
	for token, want := range map[string]int{"": 401, "guess": 401, "s3cret-token": 200} {
		if code, _ := srv.raw("GET", "/covers/"+s.ID+"?token="+token, nil); code != want {
			t.Errorf("cover with ?token=%q = %d, want %d", token, code, want)
		}
	}
}

func TestE2E_CrossOriginWritesAreRefused(t *testing.T) {
	w := newWorld()
	grandFinale(t, w)
	srv := startServer(t, Config{DataDir: t.TempDir()}, w, ServeOptions{})
	var e apiError
	if code := srv.call("POST", "/series", map[string]any{"url": gfURL}, &e, "Origin", "http://evil.example"); code != 403 || e.Error.Code != "forbidden_origin" {
		t.Errorf("cross-origin POST = %d %s", code, e.Error.Code)
	}
	if code := srv.call("GET", "/series", nil, nil, "Origin", "http://evil.example"); code != 200 {
		t.Errorf("cross-origin GET = %d (reads are allowed; CORS keeps the answer away)", code)
	}
	host := strings.TrimSuffix(strings.TrimPrefix(srv.base, "http://"), "/api/v1")
	var s apiSeries
	if code := srv.call("POST", "/series", map[string]any{"url": gfURL}, &s, "Origin", "http://"+host); code != 201 {
		t.Errorf("same-origin POST = %d", code)
	}
	var list struct {
		Series []apiSeries `json:"series"`
	}
	if srv.call("GET", "/series", nil, &list); len(list.Series) != 1 {
		t.Errorf("series = %d, want only the same-origin one", len(list.Series))
	}
}

// App mode listens on 127.0.0.1 only and refuses other Host names, so a
// web page cannot reach it by DNS rebinding.
func TestE2E_AppModeRefusesForeignHosts(t *testing.T) {
	srv := startServer(t, Config{DataDir: t.TempDir()}, newWorld(), ServeOptions{AppMode: true})
	var e apiError
	if code := srv.call("GET", "/series", nil, &e, "Host", "evil.example"); code != 403 || e.Error.Code != "forbidden_host" {
		t.Errorf("foreign Host = %d %s", code, e.Error.Code)
	}
	if code := srv.call("GET", "/series", nil, nil, "Host", "localhost"); code != 200 {
		t.Errorf("localhost = %d", code)
	}
}

func TestE2E_HostileURLsAndAnswers(t *testing.T) {
	w := newWorld()
	w.redirect["https://loop.test/a"] = "https://loop.test/a"
	w.redirect["https://jump.test/x"] = "file:///etc/passwd"
	w.set("https://huge.test/x", bytes.Repeat([]byte("<p>chapter 1</p>"), 6<<20/16))
	srv := startServer(t, Config{DataDir: t.TempDir()}, w, ServeOptions{})
	tests := []struct{ url, code string }{
		{"file:///etc/passwd", "invalid_input"},
		{"javascript:alert(1)", "invalid_input"},
		{"ftp://site.org/x", "invalid_input"},
		{"https://loop.test/a", "upstream_error"},
		{"https://jump.test/x", "invalid_input"},
		{"https://huge.test/x", "upstream_too_large"},
		{"https://nowhere.test/manga/x", "not_found"},
		{"https://down.test/manga/x", "upstream_error"},
	}
	for _, tt := range tests {
		var e apiError
		code := srv.call("POST", "/extract/preview", map[string]string{"url": tt.url}, &e)
		if code < 400 || e.Error.Code != tt.code {
			t.Errorf("preview %s = %d %q, want %s", tt.url, code, e.Error.Code, tt.code)
		}
	}
	if n := strings.Count(strings.Join(w.requests, "\n"), "loop.test"); n > 20 {
		t.Errorf("the redirect loop was followed %d times", n)
	}
}

func TestE2E_CoverUploadOnlyTakesImages(t *testing.T) {
	w := newWorld()
	grandFinale(t, w)
	srv := startServer(t, Config{DataDir: t.TempDir()}, w, ServeOptions{})
	var s apiSeries
	srv.call("POST", "/series", map[string]any{"url": gfURL}, &s)
	for body, want := range map[string]int{"<script>alert(1)</script>": 400, string(tinyPNG): 200} {
		if code, _ := srv.raw("PUT", "/series/"+s.ID+"/cover", strings.NewReader(body)); code != want {
			t.Errorf("upload %.10q = %d, want %d", body, code, want)
		}
	}
}

// The apps listen on a fixed port so the page keeps its origin (and the
// preferences stored for it); a taken port falls back to a free one.
func TestListen_FallsBackWhenThePortIsTaken(t *testing.T) {
	taken, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()
	port := taken.Addr().(*net.TCPAddr).Port
	logger := slog.New(slog.DiscardHandler)
	if _, err := listen(context.Background(), "127.0.0.1", port, false, logger); err == nil {
		t.Error("a taken port without fallback must fail")
	}
	ln, err := listen(context.Background(), "127.0.0.1", port, true, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	if ln.Addr().(*net.TCPAddr).Port == port {
		t.Error("the fallback reused the taken port")
	}
}

// Two LeftOffs on one data folder would overwrite each other's
// library (an old AppImage did, on 2026-10-21): the second one stops.
func TestServe_RefusesADataFolderInUse(t *testing.T) {
	dir := t.TempDir()
	startServer(t, Config{DataDir: dir}, newWorld(), ServeOptions{Version: "first"})
	err := Serve(context.Background(), Config{DataDir: dir, Port: 0}, slog.New(slog.DiscardHandler), ServeOptions{Host: "127.0.0.1", AppMode: true})
	if !errors.Is(err, storage.ErrLocked) || !strings.Contains(err.Error(), "server first") {
		t.Errorf("second server: %v", err)
	}
}
