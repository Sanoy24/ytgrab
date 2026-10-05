package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/Sanoy24/ytgrab/internal/activity"
	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/autostart"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/cooldown"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	"github.com/Sanoy24/ytgrab/internal/phone"
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
	// An installed update ends the run like Quit, then reports ErrRestart.
	ctx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	var restarting atomic.Bool
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
	appSettings.SetCookieFile(filepath.Join(cfg.DataDir, "cookies.txt"))
	jobQueue := queue.New(store, ytdlp.Downloader{Config: cfg, DownloadsDir: appSettings.DownloadsDir, CookiesBrowser: appSettings.CookiesSource, Subtitles: appSettings.Subtitles, SpeedLimit: appSettings.SpeedLimit, FileNames: appSettings.FileNames, SponsorBlock: appSettings.SponsorBlock, NormalizeAudio: appSettings.NormalizeAudio, SaveMetadata: appSettings.SaveMetadata}, 2)
	// One pause gate for downloads and format checks: when YouTube limits this network,
	// everything waits instead of retrying into a longer block.
	jobQueue.SetCooldown(cooldown.New())
	jobQueue.SetLimit(appSettings.MaxDownloads)
	jobQueue.SetWindow(appSettings.DownloadWindow)
	appSettings.OnWindowChange(jobQueue.Wake)
	jobQueue.Start(ctx)
	defer jobQueue.Stop()
	if cfg.Activity != nil {
		list := func(ctx context.Context) ([]domain.Job, error) { return store.List(ctx, 200) }
		go activity.Watch(ctx, 2*time.Second, list, api.PageOpen, cfg.Activity)
	}
	latest := newLatestYtdlp(appSettings.YtdlpChannel)
	latest.Get() // look up the newest yt-dlp now, so the first page load can show it
	updater := &ytdlpUpdater{cfg: cfg, running: jobQueue.Running, latest: latest, channel: appSettings.YtdlpChannel}
	go ytdlpAutoUpdate{
		enabled:  appSettings.AutoUpdateYtdlp,
		running:  jobQueue.Running,
		versions: installedAndLatest(updater),
		update:   updater.UpdateYtdlp,
		output:   output,
	}.run(ctx)
	ytgrab := ytgrabUpdatesFor(cfg.Version)
	ytgrab.running = jobQueue.Running
	ytgrab.restart = func() {
		restarting.Store(true)
		stopRun()
	}
	serving = true
	err = serve(ctx, cfg, output, listener, store, jobQueue, serverSettings{Manager: appSettings, Picker: picker.New(), ytdlpUpdater: updater, ytgrabUpdates: ytgrab, loginStart: loginStart{autostart.ForThisProgram()}})
	if err == nil && restarting.Load() {
		return ErrRestart
	}
	return err
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
	phones  *phone.Service
}

// SetYtdlpChannel saves the channel, then installs that channel's latest yt-dlp in the
// background: switching back to stable replaces a newer nightly with the stable build.
// While downloads run the install is refused; automatic updates pick it up later.
func (s serverSettings) SetYtdlpChannel(ctx context.Context, channel string) error {
	if err := s.Manager.SetYtdlpChannel(ctx, channel); err != nil {
		return err
	}
	if s.ytdlpUpdater != nil {
		s.ytdlpUpdater.latest.forget()
		go func() {
			installCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_, _ = s.ytdlpUpdater.UpdateYtdlp(installCtx)
		}()
	}
	return nil
}

// Watches offers the watched channels and playlists to the API (nil until serve starts it).
func (s serverSettings) Watches() api.Watcher {
	if s.watches == nil {
		return nil
	}
	return s.watches
}

// Phones offers phone access to the API (nil until serve starts it).
func (s serverSettings) Phones() api.PhoneAccess {
	if s.phones == nil {
		return nil
	}
	return s.phones
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
		if cookies, ok := settings[0].(interface{ CookiesSource() string }); ok {
			inspector.CookiesBrowser = cookies.CookiesSource
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
	// Phones on the same Wi-Fi reach the same pages through their own server, one port up,
	// which runs only while phone access is on.
	var phones *phone.Service
	if full, ok := store.(phone.Store); ok && len(settings) != 0 {
		if app, ok := settings[0].(serverSettings); ok {
			service, err := phone.New(ctx, full, listener.Addr().(*net.TCPAddr).Port+1)
			if err != nil {
				return err
			}
			phones, app.phones = service, service
			settings[0] = app
		}
	}
	latest := func() string { return "" }
	if len(settings) != 0 {
		if source, ok := settings[0].(interface{ LatestYtdlp() string }); ok {
			latest = source.LatestYtdlp
		}
	}
	handler := api.NewHandlerWithInspector(func(requestCtx context.Context) deps.Report {
		report := deps.Check(requestCtx, cfg)
		deps.MarkOutdated(&report, latest(), time.Now())
		return report
	}, store, controller, inspector, settings...)
	server := &http.Server{
		Handler:           identify(cfg.Version, api.RequireLoopbackHost(handler)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	if phones != nil {
		phones.Start(identify(cfg.Version, handler))
		defer phones.Close()
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
