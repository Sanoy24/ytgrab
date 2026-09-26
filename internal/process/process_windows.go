//go:build windows

package process

import (
	"os"
	"os/exec"
	"strconv"
)

func prepare(_ *exec.Cmd) {}

func terminate(process *os.Process) {
	// taskkill /T includes yt-dlp's ffmpeg child when a job is cancelled.
	if err := exec.Command("taskkill", "/PID", strconv.Itoa(process.Pid), "/T", "/F").Run(); err != nil {
		_ = process.Kill()
	}
}
