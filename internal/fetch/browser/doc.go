// Package browser solves anti-bot challenges (Cloudflare "Just a moment…")
// and renders JavaScript pages. It drives a local browser (solver.go, build
// tag rod) or a tab the host app controls, the Android WebView (driven.go).
// The local browser is a Chromium-family browser installed on this
// computer (Chrome, Chromium, Edge or Brave, also as Flatpak; RF-56). It
// first tries without a window; if the challenge needs a human, it opens a
// visible window and waits for the user to pass it.
//
// The implementation needs the "rod" build tag (go-rod); without it, New
// returns a solver that always reports it is unavailable. It does NOT
// download a browser.
package browser

import "time"

// Options configures the browser solver.
type Options struct {
	Bin         string        // browser executable (BROWSER_BIN); found automatically when empty
	ProfileDir  string        // persistent profile, e.g. <DATA_DIR>/browser-profile
	Interactive bool          // allow opening a visible window when headless fails
	Timeout     time.Duration // how long a visible window waits for the user; default 2 min
}
