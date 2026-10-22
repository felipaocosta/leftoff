package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

func TestHostGates_SpacesRequestsToSameHost(t *testing.T) {
	g := newHostGates(30*time.Millisecond, domain.SystemClock{})
	ctx := context.Background()
	start := time.Now()
	for range 3 {
		release, err := g.acquire(ctx, "a.org")
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Errorf("3 requests took %v, want >= 60ms (2 intervals)", elapsed)
	}
	// A different host is not delayed by a.org.
	other := time.Now()
	release, err := g.acquire(ctx, "B.org")
	if err != nil {
		t.Fatal(err)
	}
	release()
	if time.Since(other) > 20*time.Millisecond {
		t.Error("other host was delayed")
	}
}

func TestHostGates_CanceledWhileWaiting(t *testing.T) {
	g := newHostGates(time.Hour, domain.SystemClock{})
	release, err := g.acquire(context.Background(), "a.org")
	if err != nil {
		t.Fatal(err)
	}
	release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := g.acquire(ctx, "a.org"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
	// The slot must have been returned: a fresh gate wait is possible again.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel2()
	if _, err := g.acquire(ctx2, "a.org"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("second wait: err = %v, want DeadlineExceeded (not a deadlock on the slot)", err)
	}
}

func TestClient_NeverRunsParallelRequestsToSameHost(t *testing.T) {
	var inFlight, maxInFlight atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := inFlight.Add(1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := New(fastOptions())
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Get(context.Background(), c, srv.URL); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if maxInFlight.Load() != 1 {
		t.Errorf("max in-flight = %d, want 1", maxInFlight.Load())
	}
}
