package settings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
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

func TestCookiesBrowserIsOptInAndAllowlisted(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := New(ctx, store, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if manager.CookiesBrowser() != "" {
		t.Fatal("browser sign-in must be off by default")
	}
	for _, bad := range []string{"netscape", "firefox --exec", "chrome:Profile 1", "FIREFOX"} {
		if err := manager.SetCookiesBrowser(ctx, bad); !errors.Is(err, ErrInvalidBrowser) {
			t.Errorf("SetCookiesBrowser(%q) = %v", bad, err)
		}
	}
	if err := manager.SetCookiesBrowser(ctx, "firefox"); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := New(ctx, store, t.TempDir(), false)
	if reloaded.CookiesBrowser() != "firefox" {
		t.Fatalf("reloaded browser = %q", reloaded.CookiesBrowser())
	}
	if err := reloaded.SetCookiesBrowser(ctx, ""); err != nil || reloaded.CookiesBrowser() != "" {
		t.Fatalf("turning sign-in off = %v, %q", err, reloaded.CookiesBrowser())
	}
}

func TestDownloadPreferences(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := New(ctx, store, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if manager.MaxDownloads() != 2 || manager.DefaultPreset() != "video-best" {
		t.Fatalf("defaults = %d, %q", manager.MaxDownloads(), manager.DefaultPreset())
	}
	for _, bad := range []int{0, 5, -1} {
		if err := manager.SetMaxDownloads(ctx, bad); !errors.Is(err, ErrInvalidPreference) {
			t.Errorf("SetMaxDownloads(%d) = %v", bad, err)
		}
	}
	if err := manager.SetDefaultPreset(ctx, "video-4k"); !errors.Is(err, ErrInvalidPreference) {
		t.Errorf("unknown preset accepted: %v", err)
	}
	if err := manager.SetMaxDownloads(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetDefaultPreset(ctx, "audio-mp3"); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := New(ctx, store, t.TempDir(), false)
	if reloaded.MaxDownloads() != 3 || reloaded.DefaultPreset() != "audio-mp3" {
		t.Fatalf("reloaded = %d, %q", reloaded.MaxDownloads(), reloaded.DefaultPreset())
	}
}

func TestSubtitlesAreOffByDefaultAndAllowlisted(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := New(ctx, store, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if mode, lang := manager.Subtitles(); mode != SubtitlesOff || lang != "en" {
		t.Fatalf("default = %q %q", mode, lang)
	}
	for _, bad := range [][2]string{{"burn", "en"}, {"embed", "xx"}, {"embed", "en.*"}, {"file", "EN"}} {
		if err := manager.SetSubtitles(ctx, bad[0], bad[1]); !errors.Is(err, ErrInvalidPreference) {
			t.Errorf("SetSubtitles(%q, %q) = %v", bad[0], bad[1], err)
		}
	}
	if err := manager.SetSubtitles(ctx, SubtitlesFile, "am"); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := New(ctx, store, t.TempDir(), false)
	if mode, lang := reloaded.Subtitles(); mode != SubtitlesFile || lang != "am" {
		t.Fatalf("reloaded = %q %q", mode, lang)
	}
}

func TestSpeedLimitIsAllowlisted(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := New(ctx, store, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if manager.SpeedLimit() != 0 {
		t.Fatal("no limit by default")
	}
	for _, bad := range []int{-1, 1, 3000, 1 << 30} {
		if err := manager.SetSpeedLimit(ctx, bad); !errors.Is(err, ErrInvalidPreference) {
			t.Errorf("SetSpeedLimit(%d) = %v", bad, err)
		}
	}
	if err := manager.SetSpeedLimit(ctx, 2000); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := New(ctx, store, t.TempDir(), false)
	if reloaded.SpeedLimit() != 2000 {
		t.Fatalf("reloaded = %d", reloaded.SpeedLimit())
	}
}

func TestAutoUpdateIsOnUntilTurnedOff(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, _ := New(ctx, store, t.TempDir(), false)
	if !manager.AutoUpdateYtdlp() {
		t.Fatal("automatic updates should be on by default")
	}
	if err := manager.SetAutoUpdateYtdlp(ctx, false); err != nil {
		t.Fatal(err)
	}
	if reloaded, _ := New(ctx, store, t.TempDir(), false); reloaded.AutoUpdateYtdlp() {
		t.Fatal("turning it off didn't stick")
	}
}

func TestFileNameStyleIsAllowlisted(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, _ := New(ctx, store, t.TempDir(), false)
	if manager.FileNames() != "title" {
		t.Fatalf("default = %q", manager.FileNames())
	}
	if err := manager.SetFileNames(ctx, "%(uploader)s/../x"); !errors.Is(err, ErrInvalidPreference) {
		t.Fatalf("arbitrary template accepted: %v", err)
	}
	if err := manager.SetFileNames(ctx, "channel-folder"); err != nil {
		t.Fatal(err)
	}
	if reloaded, _ := New(ctx, store, t.TempDir(), false); reloaded.FileNames() != "channel-folder" {
		t.Fatal("not saved")
	}
}
