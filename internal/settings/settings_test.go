package settings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	sqlitestore "ytgrab/internal/store/sqlite"
)

func TestOutputFolderPersistsAndValidates(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := New(ctx, store, filepath.Join(t.TempDir(), "default"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetDownloadsDir(ctx, "relative"); !errors.Is(err, ErrInvalidDirectory) {
		t.Fatalf("relative directory error = %v", err)
	}
	if err := manager.SetDownloadsDir(ctx, filepath.Join(t.TempDir(), "missing")); !errors.Is(err, ErrInvalidDirectory) {
		t.Fatalf("missing directory error = %v", err)
	}
	selected := t.TempDir()
	if err := manager.SetDownloadsDir(ctx, selected); err != nil {
		t.Fatal(err)
	}
	loaded, err := New(ctx, store, "other-default")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DownloadsDir() != selected {
		t.Fatalf("loaded directory = %q", loaded.DownloadsDir())
	}
}
