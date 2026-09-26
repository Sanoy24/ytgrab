package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ytgrab/internal/config"
	"ytgrab/internal/domain"
	"ytgrab/internal/downloader/ytdlp"
	"ytgrab/internal/queue"
	sqlitestore "ytgrab/internal/store/sqlite"
)

// TestIntegrationDownload is opt-in because it downloads a real video and
// depends on YouTube and a local yt-dlp binary.
func TestIntegrationDownload(t *testing.T) {
	if os.Getenv("YTGRAB_INTEGRATION") != "1" {
		t.Skip("set YTGRAB_INTEGRATION=1 to run a real download")
	}
	toolsDir, err := filepath.Abs(filepath.Join("..", "..", "tools"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ToolsDir:        toolsDir,
		DataDir:         t.TempDir(),
		DownloadsDir:    t.TempDir(),
		ShutdownTimeout: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := sqlitestore.Open(ctx, filepath.Join(cfg.DataDir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	jobQueue := queue.New(store, ytdlp.Downloader{Config: cfg}, 2)
	jobQueue.Start(ctx)
	defer jobQueue.Stop()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- serve(ctx, cfg, io.Discard, listener, store, jobQueue) }()
	defer func() {
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("server shutdown: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("server did not stop")
		}
	}()
	base := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 10 * time.Second}
	response, err := client.Post(base+"/api/jobs", "application/json", bytes.NewBufferString(`{"url":"https://www.youtube.com/watch?v=jNQXAC9IVRw","preset":"audio-m4a"}`))
	if err != nil {
		t.Fatal(err)
	}
	var job domain.Job
	if err := json.NewDecoder(response.Body).Decode(&job); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", response.StatusCode)
	}
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(base + "/api/jobs/" + job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewDecoder(response.Body).Decode(&job); err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if job.State == domain.Completed {
			if job.OutputPath == nil || job.Title == nil {
				t.Fatalf("completed job lacks title or output: %+v", job)
			}
			if job.Progress == nil || job.Progress.DownloadedBytes == 0 {
				t.Fatalf("completed job lacks parsed progress: %+v", job.Progress)
			}
			info, err := os.Stat(*job.OutputPath)
			if err != nil || info.Size() == 0 {
				t.Fatalf("output file = %v, %v", info, err)
			}
			t.Logf("downloaded %s (%d bytes)", *job.OutputPath, info.Size())
			return
		}
		if job.State == domain.Failed || job.State == domain.Cancelled {
			t.Fatalf("download ended in %s: %+v", job.State, job.Error)
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("download did not finish; last state: %s", job.State)
}
