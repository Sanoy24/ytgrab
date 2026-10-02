//go:build !windows && !linux

package autostart

func (e Entry) Supported() bool { return false }
func (e Entry) Enabled() bool   { return false }
func (e Entry) Set(bool) error  { return ErrUnsupported }
func (e Entry) Refresh() error  { return nil }

// Label names the setting.
func Label() string { return "Start when you log in" }
