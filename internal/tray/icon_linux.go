//go:build linux

package tray

import _ "embed"

// Linux tray hosts take PNG.
//
//go:embed ytgrab-tray.png
var icon []byte
