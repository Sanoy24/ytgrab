//go:build windows

package tray

import _ "embed"

// Windows tray icons are .ico files.
//
//go:embed ytgrab.ico
var icon []byte
