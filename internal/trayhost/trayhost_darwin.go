//go:build darwin && cgo

package trayhost

// The macOS menu bar always has room for status items. The icon needs cgo; builds
// without it run without one.
func available() bool { return true }
