package queue

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/cooldown"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// blockedDownloader reports a YouTube block for the first `blocks` calls.
type blockedDownloader struct {
	calls  atomic.Int32
	blocks int32
}

func (downloader *blockedDownloader) Download(context.Context, domain.Job, func(ytdlp.Event) error) (ytdlp.Result, error) {
	if downloader.calls.Add(1) <= downloader.blocks {
		return ytdlp.Result{}, &ytdlp.Error{Code: "blocked", Message: "YouTube is limiting requests from this network. Wait a while, then retry."}
	}
	return ytdlp.Result{OutputPath: "verified-by-fake"}, nil
}

func noSpacing() time.Duration { return 0 }

func newQueuedJob(t *testing.T, ctx context.Context) (*sqlitestore.Store, domain.Job) {
	t.Helper()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	job, err := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.AudioM4A)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	return store, job
}

func waitFor(t *testing.T, what string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestBlockedJobWaitsForCooldownThenResumes(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	store, job := newQueuedJob(t, ctx)
	fake := &blockedDownloader{blocks: 1}
	gate := cooldown.WithSteps(time.Hour)
	queue := New(store, fake, 1)
	queue.SetCooldown(gate)
	queue.spacing = noSpacing
	queue.Start(ctx)
	defer queue.Stop()

	waitFor(t, "the blocked job to return to the queue", func() bool {
		current, _ := store.Get(ctx, job.ID)
		return current.State == domain.Queued && current.Attempt == 2
	})
	if until, paused := queue.PausedUntil(); !paused || time.Until(until) < 50*time.Minute {
		t.Fatalf("queue not paused: %v %v", until, paused)
	}
	time.Sleep(150 * time.Millisecond)
	if calls := fake.calls.Load(); calls != 1 {
		t.Fatalf("downloader ran %d times during the pause", calls)
	}

	queue.Resume()
	waitFor(t, "the job to complete after Resume", func() bool {
		current, _ := store.Get(ctx, job.ID)
		return current.State == domain.Completed
	})
	if _, paused := queue.PausedUntil(); paused {
		t.Fatal("queue still paused after Resume")
	}
}

func TestJobStopsAfterThreeBlockedAttempts(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	store, job := newQueuedJob(t, ctx)
	fake := &blockedDownloader{blocks: 100}
	queue := New(store, fake, 1)
	queue.SetCooldown(cooldown.WithSteps(10 * time.Millisecond))
	queue.spacing = noSpacing
	queue.Start(ctx)
	defer queue.Stop()

	waitFor(t, "the job to fail after three attempts", func() bool {
		current, _ := store.Get(ctx, job.ID)
		return current.State == domain.Failed
	})
	current, _ := store.Get(ctx, job.ID)
	if current.Attempt != 3 || current.Error == nil || current.Error.Code != "blocked" || fake.calls.Load() != 3 {
		t.Fatalf("final job = attempt %d, error %+v, calls %d", current.Attempt, current.Error, fake.calls.Load())
	}
}

func TestStartsAreSpacedOut(t *testing.T) {
	queue := New(nil, nil, 2)
	queue.SetCooldown(cooldown.New())
	queue.spacing = func() time.Duration { return 80 * time.Millisecond }
	queue.ctx = context.Background()
	started := time.Now()
	for range 3 {
		if !queue.pace() {
			t.Fatal("pace stopped")
		}
	}
	if elapsed := time.Since(started); elapsed < 150*time.Millisecond {
		t.Fatalf("three starts took %v; want them spaced about 80ms apart", elapsed)
	}
	for range 20 {
		if gap := jitteredSpacing(); gap < 3*time.Second || gap >= 8*time.Second {
			t.Fatalf("jitteredSpacing = %v, want 3s–8s", gap)
		}
	}
}
