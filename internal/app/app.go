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

	"ytgrab/internal/api"
	"ytgrab/internal/app/deps"
	"ytgrab/internal/config"
	"ytgrab/internal/downloader/ytdlp"
	"ytgrab/internal/picker"
	"ytgrab/internal/queue"
	"ytgrab/internal/settings"
	sqlitestore "ytgrab/internal/store/sqlite"
)

// Run serves the local API until ctx is cancelled, then drains active requests.
func Run(ctx context.Context, cfg config.Config, output io.Writer) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
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
	jobQueue := queue.New(store, ytdlp.Downloader{Config: cfg, DownloadsDir: appSettings.DownloadsDir}, 2)
	jobQueue.Start(ctx)
	defer jobQueue.Stop()
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddress, err)
	}
	return serve(ctx, cfg, output, listener, store, jobQueue, settingsWithPicker{Manager: appSettings, Picker: picker.New()})
}

// settingsWithPicker adds the desktop folder window to the settings routes.
type settingsWithPicker struct {
	*settings.Manager
	picker.Picker
}

func serve(ctx context.Context, cfg config.Config, output io.Writer, listener net.Listener, store api.JobStore, controller api.JobController, settings ...api.Settings) error {
	server := &http.Server{
		Handler: api.RequireLoopbackHost(api.NewHandlerWithInspector(func(requestCtx context.Context) deps.Report {
			return deps.Check(requestCtx, cfg)
		}, store, controller, ytdlp.NewInspector(cfg), settings...)),
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
