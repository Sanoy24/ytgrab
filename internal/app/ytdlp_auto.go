package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/app/deps"
)

// ytdlpAutoUpdate keeps yt-dlp current: YouTube changes often, and an old yt-dlp is the
// usual reason downloads stop working. It updates only between downloads.
type ytdlpAutoUpdate struct {
	enabled  func() bool
	running  func() int
	versions func(context.Context) (current, latest string) // "" when unknown
	update   func(context.Context) (api.YtdlpUpdate, error)
	output   io.Writer
}

// step updates yt-dlp if that is on, a newer release is known, and nothing is downloading.
// It reports what it did, for the log and tests.
func (auto ytdlpAutoUpdate) step(ctx context.Context) string {
	if !auto.enabled() {
		return "off"
	}
	if auto.running() > 0 {
		return "busy"
	}
	current, latest := auto.versions(ctx)
	if current == "" || latest == "" || current >= latest {
		return "current"
	}
	result, err := auto.update(ctx)
	switch {
	case errors.Is(err, api.ErrDownloadsRunning), errors.Is(err, api.ErrUpdateBusy):
		return "busy"
	case err != nil:
		_, _ = fmt.Fprintf(auto.output, "Automatic yt-dlp update failed: %v\n", err)
		return "failed"
	case result.Updated:
		_, _ = fmt.Fprintf(auto.output, "Updated yt-dlp from %s to %s.\n", result.Previous, result.Version)
	}
	return "updated"
}

// run checks a couple of minutes after start, then every half hour, until ctx ends.
func (auto ytdlpAutoUpdate) run(ctx context.Context) {
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		auto.step(ctx)
		timer.Reset(30 * time.Minute)
	}
}

// installedAndLatest returns the yt-dlp YTGrab would use and the newest release, either
// "" when unknown (not installed, or the release check hasn't succeeded).
func installedAndLatest(updater *ytdlpUpdater) func(context.Context) (string, string) {
	return func(ctx context.Context) (string, string) {
		latest := updater.latest.Get()
		path, err := deps.Find(updater.cfg, "yt-dlp")
		if err != nil {
			return "", latest
		}
		current, err := deps.Version(ctx, path)
		if err != nil {
			return "", latest
		}
		return current, latest
	}
}
