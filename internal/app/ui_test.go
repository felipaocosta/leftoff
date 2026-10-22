package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestRootHandler_ManifestContentType(t *testing.T) {
	ui := fstest.MapFS{"manifest.webmanifest": {Data: []byte(`{"name":"x"}`)}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/manifest.webmanifest", nil)
	newRootHandler(http.NotFoundHandler(), ui, true).ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/manifest+json" {
		t.Errorf("got %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestRootHandler(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":           {Data: []byte("<html>ui</html>")},
		"assets/app-1.js":      {Data: []byte("console.log(1)")},
		"manifest.webmanifest": {Data: []byte(`{"name":"x"}`)},
	}
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("api")) })
	tests := []struct {
		name, path  string
		built       bool
		status      int
		body, cache string
	}{
		{"index", "/", true, 200, "<html>ui</html>", "no-cache"},
		{"hashed asset", "/assets/app-1.js", true, 200, "console.log(1)", "immutable"},
		{"api", "/api/v1/health", true, 200, "api", ""},
		{"not built", "/", false, 503, "UI not built", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), "GET", tt.path, nil)
			newRootHandler(apiHandler, ui, tt.built).ServeHTTP(rec, req)
			if rec.Code != tt.status || !strings.Contains(rec.Body.String(), tt.body) {
				t.Errorf("got %d %q", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Header().Get("Cache-Control"), tt.cache) {
				t.Errorf("cache-control = %q, want %q", rec.Header().Get("Cache-Control"), tt.cache)
			}
			if tt.path != "/api/v1/health" && !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
				t.Error("UI responses need a CSP")
			}
		})
	}
}
