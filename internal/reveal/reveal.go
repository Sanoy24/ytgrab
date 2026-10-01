// Package reveal shows a downloaded file in the system's file manager.
package reveal

import (
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Show opens the file manager with path selected (on Linux file managers without the
// FileManager1 interface, the containing folder opens instead). path must be absolute;
// callers pass a path YTGrab saved, never one from the browser.
func Show(path string) error {
	path = filepath.Clean(path) // normalizes separators; Explorer's /select needs backslashes
	switch runtime.GOOS {
	case "windows":
		return startWindows(windowsCommandLine(path))
	case "darwin":
		return start(macCommand(path))
	default:
		if err := exec.Command(linuxShowItems(path)[0], linuxShowItems(path)[1:]...).Run(); err == nil {
			return nil
		}
		return start(linuxOpenFolder(path))
	}
}

// windowsCommandLine is passed verbatim: Explorer parses /select, itself and needs the
// path quoted after the comma. Windows paths cannot contain quote characters.
func windowsCommandLine(path string) string {
	return `explorer.exe /select,"` + path + `"`
}

func macCommand(path string) []string {
	return []string{"open", "-R", path}
}

func linuxShowItems(path string) []string {
	uri := (&url.URL{Scheme: "file", Path: path}).String()
	return []string{"dbus-send", "--session", "--print-reply", "--dest=org.freedesktop.FileManager1", "--type=method_call",
		"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems",
		"array:string:" + uri, "string:"}
}

func linuxOpenFolder(path string) []string {
	return []string{"xdg-open", filepath.Dir(path)}
}

func start(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
