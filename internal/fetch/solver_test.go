package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// challengeSite answers with a Cloudflare challenge unless the request
// carries the clearance cookie and the matching User-Agent.
func challengeSite(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		ck, err := r.Cookie("cf_clearance")
		if err != nil || ck.Value != "ok" || r.UserAgent() != "SolverBrowser/1" {
			w.Header().Set("Cf-Mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("<title>Just a moment...</title>"))
			return
		}
		_, _ = w.Write([]byte("<html>page " + r.URL.Path + "</html>"))
	}))
}

type fakeSolver struct {
	name  string
	err   error
	calls atomic.Int32
}

func (f *fakeSolver) Name() string { return f.name }

func (f *fakeSolver) Solve(_ context.Context, req *Request) (*Solution, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	u, _ := url.Parse(req.URL)
	return &Solution{
		Response: &Response{URL: u, StatusCode: 200, Header: http.Header{}, Body: []byte("<html>solved</html>")},
		Clearance: Clearance{
			Cookies:   []*http.Cookie{{Name: "cf_clearance", Value: "ok"}},
			UserAgent: "SolverBrowser/1",
		},
	}, nil
}

func TestClient_SolvesChallengeAndReusesClearance(t *testing.T) {
	var hits atomic.Int32
	site := challengeSite(t, &hits)
	defer site.Close()
	broken := &fakeSolver{name: "broken", err: errors.New("no browser")}
	good := &fakeSolver{name: "good"}
	opts := fastOptions()
	opts.Solvers = []Solver{broken, good}
	c := New(opts)
	ctx := context.Background()
	if got := strings.Join(c.SolverNames(), ","); got != "broken,good" {
		t.Errorf("solver names = %q", got)
	}

	resp, err := Get(ctx, c, site.URL+"/series/a")
	if err != nil || string(resp.Body) != "<html>solved</html>" {
		t.Fatalf("first request: %v %v", err, resp)
	}
	if broken.calls.Load() != 1 || good.calls.Load() != 1 {
		t.Errorf("solver calls: broken=%d good=%d", broken.calls.Load(), good.calls.Load())
	}

	// The next request to the same host replays the clearance directly.
	resp, err = Get(ctx, c, site.URL+"/series/b")
	if err != nil || string(resp.Body) != "<html>page /series/b</html>" {
		t.Fatalf("second request: %v %v", err, resp)
	}
	if good.calls.Load() != 1 {
		t.Error("clearance should avoid solving again")
	}
}

func TestClient_AllSolversFailKeepsBlocked(t *testing.T) {
	var hits atomic.Int32
	site := challengeSite(t, &hits)
	defer site.Close()
	opts := fastOptions()
	opts.Solvers = []Solver{&fakeSolver{name: "s1", err: errors.New("timeout")}}
	_, err := Get(context.Background(), New(opts), site.URL)
	if !errors.Is(err, domain.ErrBlocked) || !strings.Contains(err.Error(), "s1: timeout") {
		t.Errorf("err = %v", err)
	}
}

func TestClearances_ParentDomain(t *testing.T) {
	c := newClearances()
	c.set("novelupdates.com", Clearance{UserAgent: "ua"})
	if cl, ok := c.get("www.NovelUpdates.com"); !ok || cl.UserAgent != "ua" {
		t.Errorf("subdomain lookup = %v %v", cl, ok)
	}
	if _, ok := c.get("other.org"); ok {
		t.Error("unrelated host matched")
	}
	if _, ok := c.get("com"); ok {
		t.Error("TLD matched")
	}
}

func TestFlareSolverr_Solve(t *testing.T) {
	var got fsRequest
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"url":"https://site.org/x","status":200,
			"response":"<html>ok</html>","userAgent":"Chrome/1",
			"cookies":[{"name":"cf_clearance","value":"v","domain":".site.org","path":"/","expiry":1800000000}]}}`))
	}))
	defer svc.Close()

	f := NewFlareSolverr(svc.URL + "/")
	sol, err := f.Solve(context.Background(), &Request{Method: http.MethodPost, URL: "https://site.org/x", Body: []byte("a=1")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmd != "request.post" || got.PostData != "a=1" || got.MaxTimeout != 60000 {
		t.Errorf("request = %+v", got)
	}
	if string(sol.Response.Body) != "<html>ok</html>" || sol.Clearance.UserAgent != "Chrome/1" ||
		len(sol.Clearance.Cookies) != 1 || sol.Clearance.Cookies[0].Expires.Year() != 2027 {
		t.Errorf("solution = %+v %+v", sol.Response, sol.Clearance)
	}
}

func TestFlareSolverr_Errors(t *testing.T) {
	tests := map[string]string{
		"error status": `{"status":"error","message":"Challenge not solved"}`,
		"not json":     `<html>bad gateway</html>`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer svc.Close()
			if _, err := NewFlareSolverr(svc.URL).Solve(context.Background(), &Request{URL: "https://a.org"}); err == nil {
				t.Error("expected error")
			}
		})
	}
	if _, err := NewFlareSolverr("http://127.0.0.1:1").Solve(context.Background(), &Request{URL: "https://a.org"}); err == nil {
		t.Error("unreachable service should fail")
	}
}
