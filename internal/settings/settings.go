package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const downloadsKey = "downloads_dir"

var ErrInvalidDirectory = errors.New("Choose an existing, writable absolute folder.")

type Store interface {
	GetSetting(context.Context, string) (string, bool, error)
	PutSetting(context.Context, string, string) error
}

type Manager struct {
	mu           sync.RWMutex
	store        Store
	downloadsDir string
}

func New(ctx context.Context, store Store, defaultDirectory string) (*Manager, error) {
	directory, found, err := store.GetSetting(ctx, downloadsKey)
	if err != nil {
		return nil, fmt.Errorf("load output folder: %w", err)
	}
	if !found {
		directory = defaultDirectory
	}
	return &Manager{store: store, downloadsDir: directory}, nil
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
	return nil
}
