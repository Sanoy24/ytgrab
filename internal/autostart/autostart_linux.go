//go:build linux

package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/trayhost"
)

// Label names the setting.
func Label() string { return "Start when you log in" }

// configDir is a variable so tests can use their own folder.
var configDir = os.UserConfigDir

func entryPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "ytgrab.desktop"), nil
}

// Supported needs a tray: started at login, YTGrab has no window, so the tray icon is how
// it is opened and quit.
func (e Entry) Supported() bool { return e.Exe != "" && trayhost.Available() }

// desktopFile starts YTGrab without --open: at login it waits quietly in the tray.
func (e Entry) desktopFile() string {
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=YTGrab\n" +
		"Comment=Start YTGrab in the tray\n" +
		"Exec=" + quoteExec(e.Exe) + "\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n"
}

// quoteExec quotes a program path for a desktop entry's Exec key. The spec undoes string
// escapes before argument quoting, so a backslash is written as four backslashes and ",
// `, and $ get two.
func quoteExec(path string) string {
	replacer := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", "$", `\\$`)
	return `"` + replacer.Replace(path) + `"`
}

func (e Entry) current() (string, bool) {
	path, err := entryPath()
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(path)
	return string(data), err == nil
}

// Enabled reports whether YTGrab starts at login.
func (e Entry) Enabled() bool {
	_, ok := e.current()
	return ok
}

// Set turns starting at login on or off.
func (e Entry) Set(on bool) error {
	if e.Exe == "" {
		return ErrUnsupported
	}
	path, err := entryPath()
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
	return os.WriteFile(path, []byte(e.desktopFile()), 0o644)
}

// Refresh points an existing entry at this copy, so it keeps working after the program
// moves or Homebrew installs a new version.
func (e Entry) Refresh() error {
	if e.Exe == "" {
		return nil
	}
	if text, ok := e.current(); ok && text != e.desktopFile() {
		return e.Set(true)
	}
	return nil
}
