//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/Sanoy24/ytgrab/internal/config"
)

// Outside Windows YTGrab runs in the terminal it was started from.
func inBackground() bool        { return false }
func startedFromShortcut() bool { return false }
func showError(string)          {}

// startInBackground starts this program again, detached from the terminal, writing its
// output to ytgrab.log in the data folder. It is used to restart after an update.
func startInBackground(cfg config.Config, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(cfg.DataDir, "ytgrab.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
