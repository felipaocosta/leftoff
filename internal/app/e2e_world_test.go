package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/testfixture"
)

// world answers every outgoing request of a test server: the series sites
// from the saved fixtures, plus fake ntfy and AniList. Nothing reaches the
// network (RULES §1.4).
type world struct {
	mu       sync.Mutex
	pages    map[string][]byte // "URL" or "POST URL" → body
	redirect map[string]string // URL → Location
	requests []string
	ntfy     chan []byte // bodies posted to the ntfy topic
}

// tinyPNG is a valid 1×1 image served for every cover.
var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\rIDATx\x9cc\xf8\xff\xff?\x00\x05\xfe\x02\xfe\xa7\x35\x81\x84\x00\x00\x00\x00IEND\xaeB`\x82")

func newWorld() *world {
	return &world{pages: map[string][]byte{}, redirect: map[string]string{}, ntfy: make(chan []byte, 16)}
}

func (w *world) set(key string, body []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pages[key] = body
}

func (w *world) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body) // in-memory body
	}
	key := req.URL.String()
	if req.Method != http.MethodGet {
		key = req.Method + " " + key
	}
	w.mu.Lock()
	w.requests = append(w.requests, key)
	page, ok := w.pages[key]
	loc, moved := w.redirect[req.URL.String()]
	w.mu.Unlock()
	switch {
	case strings.HasPrefix(req.URL.Host, "down."):
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	case req.URL.Host == "ntfy.test":
		w.ntfy <- body
		return answer(req, http.StatusOK, []byte(`{}`), nil), nil
	case moved:
		return answer(req, http.StatusFound, nil, http.Header{"Location": {loc}}), nil
	case ok:
		return answer(req, http.StatusOK, page, nil), nil
	case strings.Contains(req.URL.Path, "cover") || strings.HasSuffix(req.URL.Path, ".jpg") || strings.HasSuffix(req.URL.Path, ".webp"):
		return answer(req, http.StatusOK, tinyPNG, http.Header{"Content-Type": {"image/png"}}), nil
	}
	return answer(req, http.StatusNotFound, []byte("not found"), nil), nil
}

func answer(req *http.Request, status int, body []byte, h http.Header) *http.Response {
	if h == nil {
		h = http.Header{}
	}
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", "text/html; charset=utf-8")
	}
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Header: h, Request: req,
		Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)),
		Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
	}
}

func fixture(t *testing.T, file string) []byte {
	t.Helper()
	b, err := testfixture.ReadFile(t, fixtures+file)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// server is a running app.Serve on 127.0.0.1 and a free port.
type server struct {
	t     *testing.T
	base  string
	token string
	stop  func()
}

// startServer runs the real server (the same wiring as production) on
// dataDir with w as the outside world.
func startServer(t *testing.T, cfg Config, w *world, so ServeOptions) *server {
	t.Helper()
	cfg.Port = 0
	if cfg.MinInterval == 0 {
		cfg.MinInterval = time.Millisecond
	}
	if cfg.MangaDexLangs == nil {
		cfg.MangaDexLangs = []string{"pt-br", "en"}
	}
	so.Host, so.Transport = "127.0.0.1", w
	ready := make(chan string, 1)
	so.OnReady = func(addr string) { ready <- addr }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, cfg, slog.New(slog.DiscardHandler), so) }()
	select {
	case addr := <-ready:
		s := &server{t: t, base: "http://" + addr + "/api/v1", token: cfg.AuthToken}
		s.stop = func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("serve: %v", err)
			}
		}
		t.Cleanup(func() { cancel() })
		return s
	case err := <-done:
		cancel()
		t.Fatalf("serve: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("server did not start")
	}
	return nil
}

// call sends a JSON request and decodes the JSON answer into out (if any).
func (s *server) call(method, path string, in any, out any, header ...string) int {
	s.t.Helper()
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			s.t.Fatal(err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, s.base+path, body)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	for i := 0; i+1 < len(header); i += 2 {
		if header[i] == "Host" {
			req.Host = header[i+1]
			continue
		}
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }() // read below
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			s.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}
	return resp.StatusCode
}

// errorCode is the API error code of an answer ({error: {code}}).
type apiError struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// eventually polls cond until it holds or the time runs out.
func eventually(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// raw sends a request without the token or JSON and returns the status and
// body; it is for covers, export and import.
func (s *server) raw(method, path string, body io.Reader) (int, string) {
	s.t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, s.base+path, body)
	if err != nil {
		s.t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }() // read below
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}
