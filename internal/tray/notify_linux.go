//go:build linux

package tray

import (
	"os"
	"path/filepath"

	"github.com/godbus/dbus/v5"
)

// notify shows a desktop notification through the freedesktop notification service,
// which every Linux desktop with a tray provides.
func notify(title, text string) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return
	}
	notifications := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	notifications.Call("org.freedesktop.Notifications.Notify", 0,
		"YTGrab", uint32(0), iconPath(), title, text, []string{}, map[string]dbus.Variant{}, int32(-1))
}

// iconPath is the icon the installer puts beside the program, if it is there.
func iconPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	path := filepath.Join(filepath.Dir(exe), "ytgrab.png")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}
