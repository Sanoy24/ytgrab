//go:build darwin && cgo

package tray

import "os/exec"

// notify shows a macOS notification. The title and text are passed as arguments to the
// script, never inserted into it.
func notify(title, text string) {
	cmd := exec.Command("osascript",
		"-e", "on run argv",
		"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
		"-e", "end run",
		title, text)
	if err := cmd.Start(); err == nil {
		go func() { _ = cmd.Wait() }()
	}
}
