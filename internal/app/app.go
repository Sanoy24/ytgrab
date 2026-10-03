package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Sanoy24/ytgrab/internal/activity"
	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/autostart"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/cooldown"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	"github.com/Sanoy24/ytgrab/internal/picker"
	"github.com/Sanoy24/ytgrab/internal/queue"
	"github.com/Sanoy24/ytgrab/internal/settings"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
	"github.com/Sanoy24/ytgrab/internal/watch"
)

// Run serves the local API until ctx is cancelled, then drains active requests.
func Run(ctx context.Context, cfg config.Config, output io.Writer) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	// Claim the port before touching the database: a second launch must not run recovery
	// or start workers against the running copy's jobs.
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		if runningYTGrab(cfg.ListenAddress) {
			return &AlreadyRunningError{URL: "http://" + cfg.ListenAddress + "/"}
		}
		return listenError(cfg.ListenAddress, err)
	}
	serving := false
	defer func() {
		if !serving {
			listener.Close() // serve owns the listener once it starts
		}
	}()
	release, err := lockDataDir(cfg.DataDir)
	if err != nil {
		return err
	}
	defer release()
	store, err := sqlitestore.Open(ctx, filepath.Join(cfg.DataDir, "jobs.db"))
	if err != nil {
		return fmt.Errorf("open job database: %w", err)
	}
	defer store.Close()
	if _, err := store.Recover(ctx); err != nil {
		return fmt.Errorf("recover interrupted jobs: %w", err)
	}
	appSettings, err := settings.New(ctx, store, cfg.DownloadsDir, cfg.DownloadsDirSet)
	if err != nil {
		return err
	}
	jobQueue := queue.New(store, ytdlp.Downloader{Config: cfg, DownloadsDir: appSettings.DownloadsDir, CookiesBrowser: appSettings.CookiesBrowser, Subtitles: appSettings.Subtitles, SpeedLimit: appSettings.SpeedLimit, FileNames: appSettings.FileNames, SponsorBlock: appSettings.SponsorBlock}, 2)
	// One pause gate for downloads and format checks: when YouTube limits this network,
	// everything waits instead of retrying into a longer block.
	jobQueue.SetCooldown(cooldown.New())
	jobQueue.SetLimit(appSettings.MaxDownloads)
	jobQueue.Start(ctx)
	defer jobQueue.Stop()
	if cfg.Activity != nil {
		list := func(ctx context.Context) ([]domain.Job, error) { return store.List(ctx, 200) }
		go activity.Watch(ctx, 2*time.Second, list, api.PageOpen, cfg.Activity)
	}
	latest := newLatestYtdlp()
	latest.Get() // look up the newest yt-dlp now, so the first page load can show it
	updater := &ytdlpUpdater{cfg: cfg, running: jobQueue.Running, latest: latest}
	go ytdlpAutoUpdate{
		enabled:  appSettings.AutoUpdateYtdlp,
		running:  jobQueue.Running,
		versions: installedAndLatest(updater),
		update:   updater.UpdateYtdlp,
		output:   output,
	}.run(ctx)
	serving = true
	return serve(ctx, cfg, output, listener, store, jobQueue, serverSettings{Manager: appSettings, Picker: picker.New(), ytdlpUpdater: updater, ytgrabUpdates: ytgrabUpdatesFor(cfg.Version), loginStart: loginStart{autostart.ForThisProgram()}})
}

// serverSettings adds the desktop folder window, yt-dlp updates, and starting at sign-in
// to the settings routes.
type serverSettings struct {
	*settings.Manager
	picker.Picker
	*ytdlpUpdater
	*ytgrabUpdates
	loginStart
	watches *watch.Service
}

// Watches offers the watched channels and playlists to the API (nil until serve starts it).
func (s serverSettings) Watches() api.Watcher {
	if s.watches == nil {
		return nil
	}
	return s.watches
}

// loginStart offers the Windows sign-in entry to the settings page.
type loginStart struct{ entry autostart.Entry }

func (l loginStart) StartAtLoginSupported() bool   { return l.entry.Supported() }
func (l loginStart) StartAtLogin() bool            { return l.entry.Enabled() }
func (l loginStart) SetStartAtLogin(on bool) error { return l.entry.Set(on) }
func (l loginStart) StartAtLoginLabel() string     { return autostart.Label() }

func serve(ctx context.Context, cfg config.Config, output io.Writer, listener net.Listener, store api.JobStore, controller api.JobController, settings ...api.Settings) error {
	inspector := ytdlp.NewInspector(cfg)
	if len(settings) != 0 {
		if cookies, ok := settings[0].(interface{ CookiesBrowser() string }); ok {
			inspector.CookiesBrowser = cookies.CookiesBrowser
		}
	}
	if shared, ok := controller.(interface{ Cooldown() *cooldown.Gate }); ok {
		inspector.Cooldown = shared.Cooldown()
	}
	// Watched channels share the inspector, so all YouTube listings go through one throttle
	// and the same pause when YouTube limits the network.
	if full, ok := store.(watch.Store); ok && len(settings) != 0 {
		if app, ok := settings[0].(serverSettings); ok {
			wake := func() {}
			if controller != nil {
				wake = controller.Wake
			}
			app.watches = watch.New(full, inspector, wake, output)
			settings[0] = app
			go app.watches.Run(ctx)
		}
	}
	latest := func() string { return "" }
	if len(settings) != 0 {
		if source, ok := settings[0].(interface{ LatestYtdlp() string }); ok {
			latest = source.LatestYtdlp
		}
	}
	server := &http.Server{
		Handler: identify(cfg.Version, api.RequireLoopbackHost(api.NewHandlerWithInspector(func(requestCtx context.Context) deps.Report {
			report := deps.Check(requestCtx, cfg)
			deps.MarkOutdated(&report, latest(), time.Now())
			return report
		}, store, controller, inspector, settings...))),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	// Progress streams run until their request context ends. Cancel request contexts
	// when shutdown starts, or an open browser tab would hold shutdown to its timeout.
	requests, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	server.BaseContext = func(net.Listener) context.Context { return requests }
	server.RegisterOnShutdown(cancelRequests)

	url := fmt.Sprintf("http://%s/", listener.Addr())
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	_, _ = fmt.Fprintf(output, "YTGrab %s listening on %s\nPress Ctrl+C to stop.\n", version, url)
	if cfg.OpenBrowser {
		if err := openBrowser(url); err != nil {
			_, _ = fmt.Fprintf(output, "Could not open a browser (%v); open %s manually.\n", err, url)
		}
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()
	if cfg.Ready != nil {
		downloadsDir := func() string { return cfg.DownloadsDir }
		if len(settings) != 0 {
			downloadsDir = settings[0].DownloadsDir
		}
		cfg.Ready(url, downloadsDir)
	}

	select {
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
		err := <-served
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
