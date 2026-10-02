//go:build darwin

package autostart

import (
	"errors"
	"html"
	"os"
	"path/filepath"

	"github.com/Sanoy24/ytgrab/internal/trayhost"
)

// Label names the setting the way macOS does.
func Label() string { return "Open at login" }

const agentLabel = "com.github.sanoy24.ytgrab"

// homeDir is a variable so tests can use their own folder.
var homeDir = os.UserHomeDir

func agentPath() (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist"), nil
}

// Supported needs the menu-bar icon: started at login, YTGrab has no window, so the icon
// is how it is opened and quit.
func (e Entry) Supported() bool { return e.Exe != "" && trayhost.Available() }

// plist starts YTGrab without --open: at login it waits quietly in the menu bar.
func (e Entry) plist() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + agentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + html.EscapeString(e.Exe) + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`
}

func (e Entry) current() (string, bool) {
	path, err := agentPath()
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	return string(data), err == nil
}

// Enabled reports whether YTGrab opens at login.
func (e Entry) Enabled() bool {
	_, ok := e.current()
	return ok
}

// Set turns opening at login on or off. macOS reads the Login Agent at the next login.
func (e Entry) Set(on bool) error {
	if e.Exe == "" {
		return ErrUnsupported
	}
	path, err := agentPath()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(e.plist()), 0o644)
}

// Refresh points an existing agent at this copy, so it keeps working after the program
// moves or Homebrew installs a new version.
func (e Entry) Refresh() error {
	if e.Exe == "" {
		return nil
	}
	if text, ok := e.current(); ok && text != e.plist() {
		return e.Set(true)
	}
	return nil
}
