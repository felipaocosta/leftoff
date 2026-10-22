//go:build linux

package tray

import "github.com/godbus/dbus/v5"

// statusNotifierHost asks the session bus whether a StatusNotifierWatcher
// (the tray host of KDE, or GNOME's AppIndicator extension) is running.
func statusNotifierHost() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }() // a one-off question
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	return err == nil && has
}
