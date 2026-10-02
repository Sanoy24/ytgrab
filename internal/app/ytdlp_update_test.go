package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/config"
)

func TestUpdateRefusedWhileDownloadsRunOrAnotherUpdateIsActive(t *testing.T) {
	updater := &ytdlpUpdater{cfg: config.Config{}, running: func() int { return 1 }, latest: newLatestYtdlp()}
	if _, err := updater.UpdateYtdlp(context.Background()); !errors.Is(err, api.ErrDownloadsRunning) {
		t.Fatalf("update during downloads = %v", err)
	}
	updater.mu.Lock()
	defer updater.mu.Unlock()
	if _, err := updater.UpdateYtdlp(context.Background()); !errors.Is(err, api.ErrUpdateBusy) {
		t.Fatalf("concurrent update = %v", err)
	}
}

func TestLatestVersionIsCachedForADay(t *testing.T) {
	calls := 0
	latest := &latestRelease{lookup: func(context.Context) (string, error) { calls++; return "2026.09.30", nil }}
	latest.refresh() // what Get starts in the background, run synchronously here
	for range 3 {
		if got := latest.Get(); got != "2026.09.30" {
			t.Fatalf("Get = %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("lookup ran %d times; want once per day", calls)
	}
}
