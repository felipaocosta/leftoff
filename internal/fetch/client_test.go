package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// fastOptions keeps tests quick: no real rate limit waits or backoffs.
func fastOptions() Options {
	return Options{MinInterval: time.Millisecond, RetryBackoff: time.Millisecond, CacheTTL: -1}
}

func TestClient_FetchSendsHeadersAndReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-ua" || r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
			t.Errorf("headers = %v", r.Header)
		}
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

	opts := fastOptions()
	opts.UserAgent = "test-ua"
	resp, err := New(opts).Fetch(context.Background(), &Request{
		URL:    srv.URL,
		Header: http.Header{"X-Requested-With": {"XMLHttpRequest"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "<html>ok</html>" || resp.StatusCode != 200 {
		t.Errorf("resp = %d %q", resp.StatusCode, resp.Body)
	}
}

func TestClient_StatusMapping(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		header  map[string]string
		body    string
		wantErr error
		wantSE  int
	}{
		{name: "not found", status: 404, wantErr: domain.ErrNotFound},
		{name: "gone", status: 410, wantErr: domain.ErrNotFound},
		{name: "cloudflare header", status: 403, header: map[string]string{"Cf-Mitigated": "challenge"}, wantErr: domain.ErrBlocked},
		{name: "challenge body", status: 503, body: "<title>Just a moment...</title>", wantErr: domain.ErrBlocked},
		{name: "plain forbidden", status: 403, wantSE: 403},
		{name: "server error after retries", status: 500, wantSE: 500},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			_, err := Get(context.Background(), New(fastOptions()), srv.URL)
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			var se *StatusError
			if tt.wantSE != 0 && (!errors.As(err, &se) || se.StatusCode != tt.wantSE) {
				t.Fatalf("err = %v, want StatusError %d", err, tt.wantSE)
			}
		})
	}
}

func TestClient_RetriesTransientErrorsThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if _, err := Get(context.Background(), New(fastOptions()), srv.URL); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3 (1 + 2 retries)", calls.Load())
	}
}

func TestClient_DoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	_, _ = Get(context.Background(), New(fastOptions()), srv.URL) // error asserted elsewhere
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestClient_RejectsNonHTTPURLs(t *testing.T) {
	c := New(fastOptions())
	for _, u := range []string{"file:///etc/passwd", "ftp://x.org/a", "/relative", "http://"} {
		if _, err := Get(context.Background(), c, u); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%q: err = %v, want ErrInvalidInput", u, err)
		}
	}
}

func TestClient_EnforcesBodyLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer srv.Close()
	opts := fastOptions()
	opts.MaxBodyBytes = 50
	if _, err := Get(context.Background(), New(opts), srv.URL); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, oversized body must not be retried", calls.Load())
	}
}

func TestClient_Redirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("final")) })
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New(fastOptions())

	resp, err := Get(context.Background(), c, srv.URL+"/hop")
	if err != nil || resp.URL.Path != "/final" {
		t.Fatalf("hop: resp=%v err=%v, want final URL /final", resp, err)
	}
	if _, err := Get(context.Background(), c, srv.URL+"/loop"); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("loop: err = %v, want redirect limit", err)
	}
	if _, err := Get(context.Background(), c, srv.URL+"/file"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("file redirect: err = %v, want ErrInvalidInput", err)
	}
}

func TestClient_CachesSuccessfulResponses(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	opts := fastOptions()
	opts.CacheTTL = time.Minute
	c := New(opts)
	ctx := context.Background()
	for range 3 {
		if _, err := Get(ctx, c, srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Fetch(ctx, &Request{Method: http.MethodPost, URL: srv.URL, Body: []byte("a=1")}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2 (one GET, one POST)", calls.Load())
	}
}

func TestClient_CanceledContextStopsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	opts := fastOptions()
	opts.RetryBackoff = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Get(ctx, New(opts), srv.URL); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("retry backoff ignored context")
	}
}
