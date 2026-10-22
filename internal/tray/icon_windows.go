//go:build windows

package tray

import _ "embed"

//go:embed icon.ico
var icon []byte

// Available reports whether a tray shows the icon: Windows always has the
// notification area.
func Available() bool { return true }
