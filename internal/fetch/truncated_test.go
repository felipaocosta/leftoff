package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A connection that drops mid-body must fail the fetch: a cut-off page
// would otherwise reach the extractors and could yield a cut title or a
// chapter label such as "Chapter 15" for chapter 151.
func TestClient_RejectsBodiesCutByADroppedConnection(t *testing.T) {
	tests := map[string]string{
		"content-length": "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: 1000\r\n\r\n<html><h1>Chapter 15",
		"chunked":        "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nTransfer-Encoding: chunked\r\n\r\n14\r\n<html><h1>Chapter 15\r\n",
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				conn, buf, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer func() { _ = conn.Close() }() // dropping the connection is the test
				_, _ = buf.WriteString(raw)
				_ = buf.Flush()
			}))
			defer srv.Close()
			resp, err := Get(context.Background(), New(fastOptions()), srv.URL)
			if err == nil {
				t.Fatalf("a cut-off body was accepted: %q", resp.Body)
			}
		})
	}
}

func TestClient_NetworkFailuresAreUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.String(), http.StatusFound) // a loop
	}))
	defer srv.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close() // connection refused
	for name, u := range map[string]string{"redirect loop": srv.URL, "connection refused": closed.URL} {
		if _, err := Get(context.Background(), New(fastOptions()), u); !errors.Is(err, ErrUnreachable) {
			t.Errorf("%s: err = %v, want ErrUnreachable", name, err)
		}
	}
}
