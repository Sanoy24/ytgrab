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
	"ytgrab/internal/queue"
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
	jobQueue := queue.New(store, ytdlp.Downloader{Config: cfg}, 2)
	jobQueue.Start(ctx)
	defer jobQueue.Stop()
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddress, err)
	}
	return serve(ctx, cfg, output, listener, store, jobQueue)
}

func serve(ctx context.Context, cfg config.Config, output io.Writer, listener net.Listener, store api.JobStore, controller api.JobController) error {
	server := &http.Server{
		Handler: api.NewHandler(func(requestCtx context.Context) deps.Report {
			return deps.Check(requestCtx, cfg)
		}, store, controller),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	_, _ = fmt.Fprintf(output, "YTGrab listening on http://%s\n", listener.Addr())
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
