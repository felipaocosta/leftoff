//go:build !windows

package tray

import _ "embed"

//go:embed icon.png
var icon []byte

// Available reports whether a tray shows the icon: on Linux, whether a
// StatusNotifierItem host runs (GNOME needs the AppIndicator extension).
// Without one, a hidden window could never come back, so the desktop app
// quits on close instead.
func Available() bool { return statusNotifierHost() }
