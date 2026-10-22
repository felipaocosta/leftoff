//go:build rod && !linux

package browser

// reapProfile is only needed for Flatpak browsers, which exist on Linux;
// elsewhere killing the launcher ends the browser.
func reapProfile(string) {}
