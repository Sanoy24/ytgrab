package app

import (
	"os/exec"
	"runtime"
)

// browserCommand returns the command that opens url in the default browser. The URL is
// always the app's own loopback address, never user input.
func browserCommand(goos string, url string) []string {
	switch goos {
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	case "darwin":
		return []string{"open", url}
	default:
		return []string{"xdg-open", url}
	}
}

func openBrowser(url string) error {
	args := browserCommand(runtime.GOOS, url)
	cmd := exec.Command(args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
