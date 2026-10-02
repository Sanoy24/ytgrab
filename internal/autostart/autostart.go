// Package autostart starts YTGrab when the user signs in. On Windows it uses the per-user
// Run registry key, on Linux an XDG autostart entry, and on macOS a Login Agent; none needs
// administrator rights.
package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupported is returned when this program can't be started at sign-in.
var ErrUnsupported = errors.New("starting at sign-in isn't supported here")

// Entry is the sign-in entry for one copy of the program.
type Entry struct {
	Exe string
}

// ForThisProgram returns the entry for the running program. A `go run` build lives in a
// temporary folder that disappears, so it gets an empty, unsupported Entry.
func ForThisProgram() Entry {
	exe, err := os.Executable()
	if err != nil || strings.Contains(filepath.ToSlash(exe), "/go-build") {
		return Entry{}
	}
	return Entry{Exe: exe}
}
