//go:build windows

package reveal

import (
	"os/exec"
	"syscall"
)

func startWindows(commandLine string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: commandLine}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Explorer exits with a non-zero code even when it succeeds, so the result is ignored.
	go func() { _ = cmd.Wait() }()
	return nil
}
