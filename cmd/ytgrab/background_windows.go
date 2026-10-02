//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Sanoy24/ytgrab/internal/config"
)

// backgroundEnv marks the copy started by startInBackground.
const backgroundEnv = "YTGRAB_BACKGROUND"

var getConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

func inBackground() bool { return os.Getenv(backgroundEnv) == "1" }

// startedFromShortcut reports that this process has its console window to itself: it was
// started from Explorer, the Start menu, or a shortcut rather than from a terminal.
func startedFromShortcut() bool {
	if inBackground() {
		return false
	}
	var ids [2]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	return n == 1
}

// startInBackground starts this program again with the same arguments but no visible
// console, writing its output to ytgrab.log in the data folder. The hidden console is
// inherited by yt-dlp and FFmpeg, so they don't open windows either.
func startInBackground(cfg config.Config, args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	logPath := filepath.Join(cfg.DataDir, "ytgrab.log")
	if info, err := os.Stat(logPath); err == nil && info.Size() > 5<<20 {
		_ = os.Rename(logPath, logPath+".old") // fails while a running copy has it open; fine
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), backgroundEnv+"=1")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// showError tells the user about a failure when there is no console to print it to.
func showError(message string) {
	text, _ := windows.UTF16PtrFromString(message)
	title, _ := windows.UTF16PtrFromString("YTGrab")
	_, _ = windows.MessageBox(0, text, title, windows.MB_OK|windows.MB_ICONERROR)
}
