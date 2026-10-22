//go:build !linux && !windows

package tray

// statusNotifierHost: other systems are not targeted (SPEC §3).
func statusNotifierHost() bool { return false }
