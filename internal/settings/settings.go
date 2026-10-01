package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	downloadsKey = "downloads_dir"
	cookiesKey   = "cookies_browser"
)

// Browsers yt-dlp can read YouTube sign-in cookies from. Only these names are accepted,
// so the setting can never become an arbitrary yt-dlp option.
var CookieBrowsers = []string{"firefox", "chrome", "edge", "brave", "chromium", "opera", "vivaldi", "safari"}

var ErrInvalidBrowser = errors.New("Choose one of the listed browsers.")

var ErrInvalidDirectory = errors.New("Choose an existing, writable absolute folder.")

type Store interface {
	GetSetting(context.Context, string) (string, bool, error)
	PutSetting(context.Context, string, string) error
}

type Manager struct {
	mu           sync.RWMutex
	store        Store
	downloadsDir string
	cookies      string
	defaultDir   string
	configured   bool
}

// New loads the saved output folder. Until the user chooses one (or sets
// YTGRAB_DOWNLOAD_DIR, reported as explicit), the page asks for it before downloading.
func New(ctx context.Context, store Store, defaultDirectory string, explicit bool) (*Manager, error) {
	directory, found, err := store.GetSetting(ctx, downloadsKey)
	if err != nil {
		return nil, fmt.Errorf("load output folder: %w", err)
	}
	if !found {
		directory = defaultDirectory
	}
	cookies, _, err := store.GetSetting(ctx, cookiesKey)
	if err != nil {
		return nil, fmt.Errorf("load sign-in setting: %w", err)
	}
	if !slices.Contains(CookieBrowsers, cookies) {
		cookies = ""
	}
	return &Manager{store: store, downloadsDir: directory, cookies: cookies, defaultDir: defaultDirectory, configured: found || explicit}, nil
}

// CookiesBrowser returns the browser whose YouTube sign-in yt-dlp should use, or "" (off).
func (manager *Manager) CookiesBrowser() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.cookies
}

// SetCookiesBrowser turns browser sign-in on for one of CookieBrowsers, or off with "".
func (manager *Manager) SetCookiesBrowser(ctx context.Context, browser string) error {
	if browser != "" && !slices.Contains(CookieBrowsers, browser) {
		return ErrInvalidBrowser
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, cookiesKey, browser); err != nil {
		return fmt.Errorf("save sign-in setting: %w", err)
	}
	manager.cookies = browser
	return nil
}

// Configured reports whether an output folder has been chosen.
func (manager *Manager) Configured() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.configured
}

// DefaultDir is the suggested folder offered on first run.
func (manager *Manager) DefaultDir() string {
	return manager.defaultDir
}

// UseDefault creates the suggested folder if needed and saves it as the choice.
func (manager *Manager) UseDefault(ctx context.Context) error {
	if err := os.MkdirAll(manager.defaultDir, 0o755); err != nil {
		return ErrInvalidDirectory
	}
	return manager.SetDownloadsDir(ctx, manager.defaultDir)
}

func (manager *Manager) DownloadsDir() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.downloadsDir
}

func (manager *Manager) SetDownloadsDir(ctx context.Context, directory string) error {
	if directory == "" || strings.TrimSpace(directory) != directory || !filepath.IsAbs(directory) {
		return ErrInvalidDirectory
	}
	directory = filepath.Clean(directory)
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return ErrInvalidDirectory
	}
	probe, err := os.CreateTemp(directory, ".ytgrab-write-check-*")
	if err != nil {
		return ErrInvalidDirectory
	}
	if err := probe.Close(); err != nil {
		_ = os.Remove(probe.Name())
		return ErrInvalidDirectory
	}
	if err := os.Remove(probe.Name()); err != nil {
		return fmt.Errorf("remove write-check file: %w", err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, downloadsKey, directory); err != nil {
		return fmt.Errorf("save output folder: %w", err)
	}
	manager.downloadsDir = directory
	manager.configured = true
	return nil
}
