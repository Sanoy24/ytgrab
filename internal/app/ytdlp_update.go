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

// latestYtdlp remembers the newest yt-dlp release, refreshed in the background at most
// once a day so health checks never wait on the network.
type latestYtdlp struct {
	mu       sync.Mutex
	version  string
	checked  time.Time
	fetching bool
	lookup   func(context.Context) (string, error)
}

func newLatestYtdlp() *latestYtdlp {
	return &latestYtdlp{lookup: func(ctx context.Context) (string, error) {
		return setup.LatestYtdlpVersion(ctx, http.DefaultClient, "")
	}}
}

// Get returns the last known latest version ("" if unknown) and starts a refresh when
// the value is more than a day old.
func (latest *latestYtdlp) Get() string {
	latest.mu.Lock()
	defer latest.mu.Unlock()
	if !latest.fetching && time.Since(latest.checked) > 24*time.Hour {
		latest.fetching = true
		go latest.refresh()
	}
	return latest.version
}

func (latest *latestYtdlp) refresh() {
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
}

func (latest *latestYtdlp) set(version string) {
	latest.mu.Lock()
	latest.version, latest.checked = version, time.Now()
	latest.mu.Unlock()
}

// ytdlpUpdater installs the latest yt-dlp into YTGrab's own tools folder, which is searched
// before PATH, so a system-wide yt-dlp is never overwritten.
type ytdlpUpdater struct {
	cfg     config.Config
	running func() int
	latest  *latestYtdlp
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
