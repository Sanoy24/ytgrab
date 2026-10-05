package settings

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// CookieFileSetting is the sign-in choice that uses an imported cookies.txt file instead of
// reading a browser. On Windows, Chrome and its relatives encrypt their sign-in data so
// yt-dlp often can't read it; a file exported from the browser always works.
const CookieFileSetting = "file"

// MaxCookieFileBytes bounds an imported cookies.txt.
const MaxCookieFileBytes = 1 << 20

var (
	ErrInvalidCookieFile = errors.New(`This isn't a cookies.txt file. Export one in Netscape format (its first line is "# Netscape HTTP Cookie File") and choose it again.`)
	ErrNoCookieFile      = errors.New("Choose a cookies.txt file first.")
)

// SetCookieFile sets where an imported cookies.txt is kept: in YTGrab's data folder.
func (manager *Manager) SetCookieFile(path string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.cookieFile = path
}

// CookiesSource is what downloads sign in with: a browser's name, the imported cookies.txt
// file's path, or "" for none.
func (manager *Manager) CookiesSource() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if manager.cookies != CookieFileSetting {
		return manager.cookies
	}
	if manager.cookieFile != "" {
		if info, err := os.Stat(manager.cookieFile); err == nil && info.Mode().IsRegular() {
			return manager.cookieFile
		}
	}
	return ""
}

// CookieFileSupported reports that a cookies.txt can be imported here.
func (manager *Manager) CookieFileSupported() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.cookieFile != ""
}

// ImportCookies checks and saves a cookies.txt (readable only by this user), then signs
// in with it.
func (manager *Manager) ImportCookies(ctx context.Context, data []byte) error {
	if !validCookieFile(data) {
		return ErrInvalidCookieFile
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.cookieFile == "" {
		return ErrNoCookieFile
	}
	if err := os.MkdirAll(filepath.Dir(manager.cookieFile), 0o700); err != nil {
		return err
	}
	staging := manager.cookieFile + ".new"
	if err := os.WriteFile(staging, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(staging, manager.cookieFile); err != nil {
		_ = os.Remove(staging)
		return err
	}
	if err := manager.store.PutSetting(ctx, cookiesKey, CookieFileSetting); err != nil {
		return err
	}
	manager.cookies = CookieFileSetting
	return nil
}

// validCookieFile accepts the Netscape format yt-dlp reads: the header line yt-dlp checks
// for, then at least one cookie of seven tab-separated fields.
func validCookieFile(data []byte) bool {
	if len(data) == 0 || len(data) > MaxCookieFileBytes || !bytes.Contains(data[:min(len(data), 200)], []byte("HTTP Cookie File")) {
		return false
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n") {
		line = strings.TrimPrefix(line, "#HttpOnly_")
		if line != "" && !strings.HasPrefix(line, "#") && len(strings.Split(line, "\t")) == 7 {
			return true
		}
	}
	return false
}

// removeCookieFile deletes the imported cookies.txt when sign-in stops using it, so no
// sign-in data is left behind. Call with mu held.
func (manager *Manager) removeCookieFile() {
	if manager.cookieFile != "" {
		_ = os.Remove(manager.cookieFile)
	}
}
