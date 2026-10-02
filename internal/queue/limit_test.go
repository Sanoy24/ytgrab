package queue

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// holdingDownloader blocks every download until the test ends, tracking concurrency.
type holdingDownloader struct {
	running, peak atomic.Int32
}

func (d *holdingDownloader) Download(ctx context.Context, _ domain.Job, _ func(ytdlp.Event) error) (ytdlp.Result, error) {
	n := d.running.Add(1)
	for {
		peak := d.peak.Load()
		if n <= peak || d.peak.CompareAndSwap(peak, n) {
			break
		}
	}
	<-ctx.Done()
	d.running.Add(-1)
	return ytdlp.Result{}, ctx.Err()
}

func TestParallelDownloadLimitAppliesImmediately(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, id := range []string{"dQw4w9WgXcQ", "jNQXAC9IVRw", "aqz-KE-bpKQ"} {
		job, _ := domain.NewJob("https://youtu.be/"+id, domain.AudioM4A)
		if err := store.Create(ctx, job); err != nil {
			t.Fatal(err)
		}
	}
	fake := &holdingDownloader{}
	var limit atomic.Int32
	limit.Store(1)
	queue := New(store, fake, 2)
	queue.SetLimit(func() int { return int(limit.Load()) })
	queue.Start(ctx)
	defer queue.Stop()

	waitFor(t, "one download", func() bool { return fake.running.Load() == 1 })
	time.Sleep(200 * time.Millisecond)
	if n := fake.running.Load(); n != 1 {
		t.Fatalf("%d downloads running with a limit of 1", n)
	}
	limit.Store(2)
	queue.Wake()
	waitFor(t, "a second download after raising the limit", func() bool { return fake.running.Load() == 2 })
	time.Sleep(200 * time.Millisecond)
	if peak := fake.peak.Load(); peak != 2 {
		t.Fatalf("peak concurrency = %d, want 2", peak)
	}
}
