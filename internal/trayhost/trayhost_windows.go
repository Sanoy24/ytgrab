//go:build windows

package trayhost

// Available is always true on Windows, whose taskbar always has a notification area.
func Available() bool { return true }
