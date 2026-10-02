//go:build !windows && !linux && !(darwin && cgo)

package trayhost

// No tray or menu-bar icon on this system or build.
func available() bool { return false }
