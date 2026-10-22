//go:build rod

package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
)

// devtools serves a /json/list whose titles change over successive polls.
func devtools(t *testing.T, lists ...string) (string, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/list" {
			http.NotFound(w, r)
			return
		}
		i := int(calls.Add(1)) - 1
		if i >= len(lists) {
			i = len(lists) - 1
		}
		_, _ = w.Write([]byte(lists[i]))
	}))
	t.Cleanup(srv.Close)
	return "ws://" + strings.TrimPrefix(srv.URL, "http://") + "/devtools/browser/x", &calls
}

const (
	challenge = `[{"id":"A","type":"page","title":"Just a moment...","url":"https://www.site.org/s/x/"}]`
	loading   = `[{"id":"A","type":"page","title":"www.site.org","url":"https://www.site.org/s/x/"}]`
	cleared   = `[{"id":"B","type":"other","title":"x","url":"chrome://x"},{"id":"A","type":"page","title":"X - Site","url":"https://www.site.org/s/x/"}]`
)

func TestWaitTabCleared_WaitsForAStableRealTitle(t *testing.T) {
	ws, calls := devtools(t, challenge, challenge, loading, cleared, cleared)
	id, err := waitTabCleared(context.Background(), ws, "www.site.org", 20*time.Second)
	if err != nil || id != "A" {
		t.Fatalf("id = %q, err = %v", id, err)
	}
	if calls.Load() < 5 {
		t.Errorf("returned after %d polls; the title must stay real for two polls", calls.Load())
	}
}

func TestWaitTabCleared_TimesOutWhileChallenged(t *testing.T) {
	ws, _ := devtools(t, challenge)
	if _, err := waitTabCleared(context.Background(), ws, "www.site.org", 1500*time.Millisecond); !errors.Is(err, errStillChallenged) {
		t.Errorf("err = %v, want errStillChallenged", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := waitTabCleared(ctx, ws, "www.site.org", time.Minute); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
}

func TestClearedTab_IgnoresOtherHostsAndTitles(t *testing.T) {
	ws, _ := devtools(t, `[{"id":"Z","type":"page","title":"Other","url":"https://other.org/"},
		{"id":"P","type":"page","title":"https://site.org/x","url":"https://site.org/x"},
		{"id":"Q","type":"page","title":"Um momento…","url":"https://site.org/x"}]`)
	list := "http://" + strings.TrimSuffix(strings.TrimPrefix(ws, "ws://"), "/devtools/browser/x") + "/json/list"
	if id := clearedTab(context.Background(), list, "www.site.org"); id != "" {
		t.Errorf("clearedTab = %q, want none", id)
	}
}

func TestMinimalFlags_KeepsOnlyWhatAWindowNeeds(t *testing.T) {
	l := launcher.New().Bin("/bin/browser").UserDataDir("/tmp/p").Set(flags.Arguments, "https://site.org/")
	minimalFlags(l)
	l.Set(flags.Arguments, "https://site.org/")
	for _, bad := range []flags.Flag{"enable-automation", "disable-site-isolation-trials", "disable-popup-blocking", "no-startup-window", flags.Headless} {
		if l.Has(bad) {
			t.Errorf("flag %s must be removed", bad)
		}
	}
	for _, want := range []flags.Flag{flags.UserDataDir, flags.RemoteDebuggingPort, flags.Arguments, "no-first-run"} {
		if !l.Has(want) {
			t.Errorf("flag %s must be kept", want)
		}
	}
}
