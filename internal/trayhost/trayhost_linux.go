//go:build linux

package trayhost

import "github.com/godbus/dbus/v5"

// Available reports whether a tray host (a StatusNotifierWatcher, as on KDE, Ubuntu, and
// GNOME with the AppIndicator extension) is running on the session bus.
func Available() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer conn.Close()
	var has bool
	err = conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&has)
	return err == nil && has
}
