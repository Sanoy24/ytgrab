//go:build !windows && !linux

package trayhost

// Available is false: YTGrab has no menu-bar icon on this system yet.
func Available() bool { return false }
