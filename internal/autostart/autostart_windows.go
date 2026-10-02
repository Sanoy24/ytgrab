//go:build windows

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// runKey is a variable so tests can use their own key.
var runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

const valueName = "YTGrab"

// Label names the setting the way Windows users know it.
func Label() string { return "Start with Windows" }

// Supported reports whether this copy can be started at sign-in.
func (e Entry) Supported() bool { return e.Exe != "" }

// command starts YTGrab without --open: at sign-in it waits quietly in the tray.
func (e Entry) command() string { return `"` + e.Exe + `"` }

func (e Entry) current() (string, bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer key.Close()
	value, _, err := key.GetStringValue(valueName)
	return value, err == nil
}

// Enabled reports whether YTGrab starts at sign-in.
func (e Entry) Enabled() bool {
	_, ok := e.current()
	return ok
}

// Set turns starting at sign-in on or off.
func (e Entry) Set(on bool) error {
	if !e.Supported() {
		return ErrUnsupported
	}
	if !on {
		key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		if err := key.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(valueName, e.command())
}

// Refresh points an existing entry at this copy, so it keeps working after the folder
// moves or Scoop installs a new version.
func (e Entry) Refresh() error {
	if !e.Supported() {
		return nil
	}
	if value, ok := e.current(); ok && value != e.command() {
		return e.Set(true)
	}
	return nil
}
