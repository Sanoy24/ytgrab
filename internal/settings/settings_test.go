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
	manager, err := New(ctx, store, filepath.Join(t.TempDir(), "default"), false)
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
	loaded, err := New(ctx, store, "other-default", false)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DownloadsDir() != selected {
		t.Fatalf("loaded directory = %q", loaded.DownloadsDir())
	}
}

func TestFolderMustBeChosenOnFirstRun(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	suggested := filepath.Join(t.TempDir(), "Downloads", "ytgrab")
	manager, err := New(ctx, store, suggested, false)
	if err != nil {
		t.Fatal(err)
	}
	if manager.Configured() || manager.DefaultDir() != suggested {
		t.Fatalf("fresh install configured=%v default=%q", manager.Configured(), manager.DefaultDir())
	}
	if err := manager.UseDefault(ctx); err != nil {
		t.Fatal(err)
	}
	if !manager.Configured() || manager.DownloadsDir() != suggested {
		t.Fatalf("after UseDefault configured=%v dir=%q", manager.Configured(), manager.DownloadsDir())
	}
	reloaded, err := New(ctx, store, "elsewhere", false)
	if err != nil || !reloaded.Configured() || reloaded.DownloadsDir() != suggested {
		t.Fatalf("reloaded configured=%v dir=%q err=%v", reloaded.Configured(), reloaded.DownloadsDir(), err)
	}

	other, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "other.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	explicit, err := New(ctx, other, t.TempDir(), true)
	if err != nil || !explicit.Configured() {
		t.Fatalf("explicit YTGRAB_DOWNLOAD_DIR should count as chosen: %v, %v", explicit.Configured(), err)
	}
}
