package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// freePort returns a port nothing listens on.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close() // only needed the number
	return port
}

func TestDetectFlareSolverr(t *testing.T) {
	fs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"msg":"FlareSolverr is ready!","version":"3.5.2"}`))
	}))
	defer fs.Close()
	if got := DetectFlareSolverr(context.Background(), fs.URL); got != fs.URL {
		t.Errorf("running FlareSolverr not detected: %q", got)
	}
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html>not it</html>`))
	}))
	defer other.Close()
	if got := DetectFlareSolverr(context.Background(), other.URL); got != "" {
		t.Errorf("another service was taken for FlareSolverr: %q", got)
	}
	if got := DetectFlareSolverr(context.Background(), "http://127.0.0.1:"+strconv.Itoa(freePort(t))); got != "" {
		t.Errorf("nothing listening: %q", got)
	}
}
