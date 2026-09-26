//go:build !windows

package process

import (
	"os"
	"os/exec"
	"syscall"
)

func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminate(process *os.Process) {
	_ = syscall.Kill(-process.Pid, syscall.SIGKILL)
}
