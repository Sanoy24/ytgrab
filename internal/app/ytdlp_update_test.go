package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/config"
)

func TestUpdateRefusedWhileDownloadsRunOrAnotherUpdateIsActive(t *testing.T) {
	updater := &ytdlpUpdater{cfg: config.Config{}, running: func() int { return 1 }, latest: newLatestYtdlp(func() string { return "stable" })}
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

func TestFreshWaitsForANewLookup(t *testing.T) {
	release := make(chan struct{})
	latest := &latestRelease{version: "1.13.0", checked: time.Now().Add(-2 * time.Hour), lookup: func(context.Context) (string, error) {
		<-release
		return "1.13.1", nil
	}}
	// Recent enough: answered from memory without a lookup.
	if got := latest.Fresh(context.Background(), 3*time.Hour); got != "1.13.0" {
		t.Fatalf("recent = %q", got)
	}
	go func() { time.Sleep(50 * time.Millisecond); close(release) }()
	if got := latest.Fresh(context.Background(), time.Hour); got != "1.13.1" {
		t.Fatalf("after waiting = %q", got)
	}
	// A slow lookup doesn't hold the page past its deadline.
	stuck := &latestRelease{version: "1.13.0", lookup: func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if got := stuck.Fresh(ctx, time.Hour); got != "1.13.0" {
		t.Fatalf("timed out = %q", got)
	}
	if newLatestYTGrab().maxAge != 6*time.Hour {
		t.Error("YTGrab releases are looked for every 6 hours")
	}
}
