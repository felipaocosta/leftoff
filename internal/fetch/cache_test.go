package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

type stepClock struct{ t time.Time }

func (c *stepClock) Now() time.Time { return c.t }

func TestResponseCache_ExpiresAfterTTL(t *testing.T) {
	clock := &stepClock{t: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}
	c := newResponseCache(time.Minute, 10, clock)
	c.put("k", &Response{StatusCode: 200})
	if _, ok := c.get("k"); !ok {
		t.Fatal("fresh entry missing")
	}
	clock.t = clock.t.Add(time.Minute)
	if _, ok := c.get("k"); ok {
		t.Error("entry should have expired")
	}
}

func TestResponseCache_EvictsWhenFull(t *testing.T) {
	clock := &stepClock{t: time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)}
	c := newResponseCache(time.Minute, 2, clock)
	c.put("a", &Response{})
	clock.t = clock.t.Add(time.Second)
	c.put("b", &Response{})
	c.put("c", &Response{}) // full: "a" expires soonest and is dropped
	if _, ok := c.get("a"); ok {
		t.Error("a should have been evicted")
	}
	for _, k := range []string{"b", "c"} {
		if _, ok := c.get(k); !ok {
			t.Errorf("%s missing", k)
		}
	}
}

func TestResponseCache_DisabledWithNonPositiveTTL(t *testing.T) {
	c := newResponseCache(-1, 10, domain.SystemClock{})
	c.put("k", &Response{})
	if _, ok := c.get("k"); ok {
		t.Error("disabled cache returned an entry")
	}
}

func TestClient_FreshContextSkipsTheCacheAndUpdatesIt(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "v%d", calls.Add(1))
	}))
	defer srv.Close()
	opts := fastOptions()
	opts.CacheTTL = time.Hour
	c := New(opts)
	ctx := context.Background()
	body := func(ctx context.Context) string {
		resp, err := Get(ctx, c, srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		return string(resp.Body)
	}
	if a, b := body(ctx), body(ctx); a != "v1" || b != "v1" {
		t.Fatalf("cached = %s %s", a, b)
	}
	if got := body(Fresh(ctx)); got != "v2" {
		t.Errorf("fresh = %s, want v2", got)
	}
	if got := body(ctx); got != "v2" {
		t.Errorf("after fresh = %s, want the fresh answer cached", got)
	}
}
