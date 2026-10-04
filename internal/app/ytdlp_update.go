package app

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/setup"
)

// latestRelease remembers the newest release of a program, refreshed in the background
// once it is older than maxAge (a day when unset), so most requests never wait.
type latestRelease struct {
	mu       sync.Mutex
	version  string
	checked  time.Time
	fetching bool
	done     chan struct{} // closed when the running refresh ends
	maxAge   time.Duration
	lookup   func(context.Context) (string, error)
}

func newLatestYtdlp() *latestRelease {
	return &latestRelease{lookup: func(ctx context.Context) (string, error) {
		return setup.LatestYtdlpVersion(ctx, http.DefaultClient, "")
	}}
}

// YTGrab runs for days in the tray, so new releases are looked for every few hours.
func newLatestYTGrab() *latestRelease {
	return &latestRelease{maxAge: 6 * time.Hour, lookup: func(ctx context.Context) (string, error) {
		return setup.LatestYTGrabVersion(ctx, http.DefaultClient, "")
	}}
}

// Get returns the last known latest version ("" if unknown) and starts a refresh when
// the value is older than maxAge.
func (latest *latestRelease) Get() string {
	maxAge := latest.maxAge
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}
	latest.mu.Lock()
	defer latest.mu.Unlock()
	latest.startRefresh(maxAge)
	return latest.version
}

// Fresh is Get for a value at most maxAge old: when it is older, Fresh waits for a new
// lookup until ctx ends, then returns what is known.
func (latest *latestRelease) Fresh(ctx context.Context, maxAge time.Duration) string {
	latest.mu.Lock()
	done := latest.startRefresh(maxAge)
	latest.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	latest.mu.Lock()
	defer latest.mu.Unlock()
	return latest.version
}

// startRefresh starts a lookup when the value is older than maxAge, and returns a channel
// that closes when the running lookup ends (nil when none runs). Call with mu held.
func (latest *latestRelease) startRefresh(maxAge time.Duration) <-chan struct{} {
	if !latest.fetching && time.Since(latest.checked) > maxAge {
		latest.fetching = true
		latest.done = make(chan struct{})
		go latest.refresh()
	}
	if !latest.fetching {
		return nil
	}
	return latest.done
}

func (latest *latestRelease) refresh() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	version, err := latest.lookup(ctx)
	latest.mu.Lock()
	defer latest.mu.Unlock()
	latest.fetching = false
	latest.checked = time.Now() // also after a failure, so an offline machine isn't retried constantly
	if err == nil {
		latest.version = version
	}
	if latest.done != nil {
		close(latest.done)
		latest.done = nil
	}
}

func (latest *latestRelease) set(version string) {
	latest.mu.Lock()
	latest.version, latest.checked = version, time.Now()
	latest.mu.Unlock()
}

// ytdlpUpdater installs the latest yt-dlp into YTGrab's own tools folder, which is searched
// before PATH, so a system-wide yt-dlp is never overwritten.
type ytdlpUpdater struct {
	cfg     config.Config
	running func() int
	latest  *latestRelease
	mu      sync.Mutex
}

// LatestYtdlp returns the newest known yt-dlp release, or "".
func (updater *ytdlpUpdater) LatestYtdlp() string {
	return updater.latest.Get()
}

func (updater *ytdlpUpdater) UpdateYtdlp(ctx context.Context) (api.YtdlpUpdate, error) {
	if !updater.mu.TryLock() {
		return api.YtdlpUpdate{}, api.ErrUpdateBusy
	}
	defer updater.mu.Unlock()
	if updater.running != nil && updater.running() > 0 {
		return api.YtdlpUpdate{}, api.ErrDownloadsRunning
	}
	current := ""
	if path, err := deps.Find(updater.cfg, "yt-dlp"); err == nil {
		current, _ = deps.Version(ctx, path)
	}
	latest, err := updater.latest.lookup(ctx)
	if err == nil {
		updater.latest.set(latest)
		if current == latest {
			return api.YtdlpUpdate{Version: current}, nil
		}
	}
	path, err := setup.InstallYtdlp(ctx, http.DefaultClient, SetupToolsDir(updater.cfg))
	if err != nil {
		return api.YtdlpUpdate{}, err
	}
	version, err := deps.Version(ctx, path)
	if err != nil {
		return api.YtdlpUpdate{}, err
	}
	return api.YtdlpUpdate{Version: version, Previous: current, Updated: version != current}, nil
}
