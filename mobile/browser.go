package mobile

import (
	"time"

	"github.com/felipaocosta/leftoff/internal/fetch/browser"
)

// Browser is the WebView tab the Android shell lends to the core, so it
// can render JavaScript pages (mangafire) and pass anti-bot checks, showing
// the tab when one needs the user (RF-56). Calls come from core threads,
// never the UI thread, and block until done. Eval returns the JSON value
// of a JavaScript expression ("null" for undefined).
type Browser interface {
	Load(url string, timeoutMs int64) error
	Eval(js string) (string, error)
	// Show puts the tab on screen for a check on host; false when the app
	// is not visible (a background check). Hide takes it away.
	Show(host string) bool
	Hide()
	Cookies(url string) string
	UserAgent() string
}

// SetBrowser lends the WebView to the core. Call it before Start (and
// CheckNow); nil takes it back. Without one, JavaScript pages fail with
// needs_browser.
func SetBrowser(b Browser) {
	mu.Lock()
	defer mu.Unlock()
	if b == nil {
		tab = nil
		return
	}
	tab = driver{b}
}

// driver adapts Browser to browser.Driver.
type driver struct{ b Browser }

func (d driver) Load(u string, timeout time.Duration) error {
	return d.b.Load(u, timeout.Milliseconds())
}
func (d driver) Eval(js string) (string, error) { return d.b.Eval(js) }
func (d driver) Show(host string) bool          { return d.b.Show(host) }
func (d driver) Hide()                          { d.b.Hide() }
func (d driver) Cookies(u string) string        { return d.b.Cookies(u) }
func (d driver) UserAgent() string              { return d.b.UserAgent() }

var _ browser.Driver = driver{}
