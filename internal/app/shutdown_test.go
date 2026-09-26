package app

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// An open progress stream (a browser tab) must not hold shutdown until its timeout.
func TestShutdownClosesOpenProgressStreams(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job, err := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.AudioM4A)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := job.Transition(domain.Downloading); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), job, domain.Queued); err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ShutdownTimeout: 3 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- serve(ctx, cfg, io.Discard, listener, store, nil) }()

	response, err := http.Get("http://" + listener.Addr().String() + "/api/jobs/" + job.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "data:") {
		t.Fatalf("first event = %q, %v", line, err)
	}

	started := time.Now()
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("serve returned %v", err)
		}
		if elapsed := time.Since(started); elapsed > time.Second {
			t.Fatalf("shutdown took %v with an open stream", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}
