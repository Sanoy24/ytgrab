//go:build darwin && cgo

package tray

import _ "embed"

//go:embed ytgrab-tray.png
var icon []byte
