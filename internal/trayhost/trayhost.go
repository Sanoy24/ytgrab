// Package trayhost reports whether this desktop can show YTGrab's tray icon.
package trayhost

import "os"

// Available reports whether to show a tray (or menu-bar) icon. YTGRAB_NO_TRAY=1 turns it
// off, for example on a machine without a desktop session.
func Available() bool {
	return os.Getenv("YTGRAB_NO_TRAY") != "1" && available()
}
