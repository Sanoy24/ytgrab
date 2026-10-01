package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// A second launch must not touch the running copy's database: no recovery (which would
// mark its downloads interrupted) and no workers (which could claim its queued jobs).
func TestSecondLaunchLeavesTheRunningCopyAlone(t *testing.T) {
	dataDir := t.TempDir()
	store, err := sqlitestore.Open(context.Background(), filepath.Join(dataDir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	job, _ := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.AudioM4A)
	if err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	_ = job.Transition(domain.Downloading)
	if err := store.Update(context.Background(), job, domain.Queued); err != nil {
		t.Fatal(err)
	}
	store.Close()

	// The "first copy": anything answering with YTGrab's identifying header.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	first := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set(identityHeader, "1.2.0")
	})}
	go first.Serve(listener)
	defer first.Close()

	cfg := config.Config{ListenAddress: listener.Addr().String(), DataDir: dataDir, DownloadsDir: t.TempDir(), ShutdownTimeout: time.Second}
	err = Run(context.Background(), cfg, io.Discard)
	var running *AlreadyRunningError
	if !errors.As(err, &running) || running.URL != "http://"+listener.Addr().String()+"/" {
		t.Fatalf("Run = %v, want AlreadyRunningError", err)
	}

	store, err = sqlitestore.Open(context.Background(), filepath.Join(dataDir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if current, _ := store.Get(context.Background(), job.ID); current.State != domain.Downloading {
		t.Fatalf("the running copy's download became %s", current.State)
	}
}

func TestPortUsedBySomethingElse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	other := &http.Server{Handler: http.NotFoundHandler()}
	go other.Serve(listener)
	defer other.Close()
	cfg := config.Config{ListenAddress: listener.Addr().String(), DataDir: t.TempDir(), DownloadsDir: t.TempDir(), ShutdownTimeout: time.Second}
	err = Run(context.Background(), cfg, io.Discard)
	var running *AlreadyRunningError
	if err == nil || errors.As(err, &running) {
		t.Fatalf("Run = %v, want a port error", err)
	}
}

func TestDataFolderLock(t *testing.T) {
	dir := t.TempDir()
	release, err := lockDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockDataDir(dir); err == nil {
		t.Fatal("a second copy locked the same data folder")
	}
	release()
	again, err := lockDataDir(dir)
	if err != nil {
		t.Fatalf("lock after release = %v", err)
	}
	again()
}
