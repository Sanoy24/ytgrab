//go:build windows

package trayhost

// The Windows taskbar always has a notification area.
func available() bool { return true }
