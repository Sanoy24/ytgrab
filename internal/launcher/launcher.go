// Package launcher adds YTGrab, with its icon, to where people open apps: ~/Applications on
// macOS (so it shows in Launchpad and Spotlight) and the applications menu on Linux. The
// install script does the same; `ytgrab app` does it for Homebrew installs, which only
// install the program.
package launcher

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed ytgrab.icns
var icns []byte

//go:embed ytgrab.png
var png []byte

// ErrUnsupported is returned on Windows, where Scoop and the zip's shortcut cover this.
var ErrUnsupported = errors.New("adding YTGrab to your apps is for macOS and Linux; on Windows, Scoop adds it to the Start menu")

// Replaced in tests.
var (
	goos    = runtime.GOOS
	homeDir = os.UserHomeDir
	refresh = refreshLaunchers
)

// StablePath is the path to start this program by that survives updates. Homebrew runs
// it from a folder named after the version (…/Cellar/ytgrab/1.18.0/bin/ytgrab), which an
// upgrade removes; its "opt" link (…/opt/ytgrab/bin/ytgrab) always points at the current one.
func StablePath(exe string) string {
	slashed := filepath.ToSlash(exe)
	if i := strings.Index(slashed, "/Cellar/ytgrab/"); i >= 0 {
		opt := filepath.FromSlash(slashed[:i] + "/opt/ytgrab/bin/ytgrab")
		if _, err := os.Stat(opt); err == nil {
			return opt
		}
	}
	return exe
}

// Install adds YTGrab to the apps, starting exe, and returns where.
func Install(exe, version string) (string, error) {
	if strings.ContainsAny(exe, "\"\\`$\n") && goos != "windows" {
		return "", fmt.Errorf("YTGrab's folder name (%s) has characters a launcher can't hold", exe)
	}
	switch goos {
	case "darwin":
		return installMac(exe, version)
	case "linux":
		return installLinux(exe)
	}
	return "", ErrUnsupported
}

// Remove takes YTGrab out of the apps again and returns what it removed ("" if nothing).
func Remove() (string, error) {
	paths, err := entryPaths()
	if err != nil {
		return "", err
	}
	removed := ""
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return "", err
		}
		if removed == "" {
			removed = path
		}
	}
	return removed, nil
}

func entryPaths() ([]string, error) {
	home, err := homeDir()
	if err != nil {
		return nil, err
	}
	switch goos {
	case "darwin":
		return []string{filepath.Join(home, "Applications", "YTGrab.app")}, nil
	case "linux":
		data := dataHome(home)
		return []string{filepath.Join(data, "applications", "ytgrab.desktop"), linuxIcon(data)}, nil
	}
	return nil, ErrUnsupported
}

// installMac writes a small app that starts YTGrab, so it opens from Launchpad and Spotlight.
// LSUIElement keeps it out of the Dock: YTGrab lives in the menu bar.
func installMac(exe, version string) (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	app := filepath.Join(home, "Applications", "YTGrab.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(app, "Contents", "Resources"), 0o755); err != nil {
		return "", err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>YTGrab</string>
	<key>CFBundleIdentifier</key>
	<string>com.github.sanoy24.ytgrab.launcher</string>
	<key>CFBundleExecutable</key>
	<string>YTGrab</string>
	<key>CFBundleIconFile</key>
	<string>ytgrab</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>` + xmlText(version) + `</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`
	files := []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644},
		{filepath.Join(app, "Contents", "MacOS", "YTGrab"), []byte("#!/bin/sh\nexec \"" + exe + "\" --open\n"), 0o755},
		{filepath.Join(app, "Contents", "Resources", "ytgrab.icns"), icns, 0o644},
	}
	for _, file := range files {
		if err := os.WriteFile(file.path, file.data, file.mode); err != nil {
			return "", err
		}
		if err := os.Chmod(file.path, file.mode); err != nil { // WriteFile keeps an old file's mode
			return "", err
		}
	}
	refresh(app)
	return app, nil
}

func installLinux(exe string) (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	data := dataHome(home)
	icon := linuxIcon(data)
	if err := os.MkdirAll(filepath.Dir(icon), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(icon, png, 0o644); err != nil {
		return "", err
	}
	apps := filepath.Join(data, "applications")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		return "", err
	}
	entry := filepath.Join(apps, "ytgrab.desktop")
	desktop := "[Desktop Entry]\nType=Application\nName=YTGrab\nGenericName=YouTube downloader\n" +
		"Comment=Download YouTube videos you are allowed to save\n" +
		"Exec=\"" + exe + "\" --open\nIcon=" + icon + "\nTerminal=false\n" +
		"Categories=Network;AudioVideo;\nStartupNotify=false\n"
	if err := os.WriteFile(entry, []byte(desktop), 0o644); err != nil {
		return "", err
	}
	refresh(apps)
	return entry, nil
}

func dataHome(home string) string {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(home, ".local", "share")
}

func linuxIcon(data string) string {
	return filepath.Join(data, "icons", "hicolor", "512x512", "apps", "ytgrab.png")
}

// refreshLaunchers tells Launchpad or the desktop's menu about the new entry now, rather
// than at the next login. Failing is fine: they find it eventually.
func refreshLaunchers(path string) {
	switch goos {
	case "darwin":
		const lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
		_ = exec.Command(lsregister, "-f", path).Run()
	case "linux":
		if tool, err := exec.LookPath("update-desktop-database"); err == nil {
			_ = exec.Command(tool, path).Run()
		}
	}
}

func xmlText(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}
