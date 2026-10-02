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

type waitingDownloader struct {
	started   chan string
	cancelled chan string
}

type retryDownloader struct{ calls atomic.Int32 }

func (downloader *retryDownloader) Download(_ context.Context, _ domain.Job, _ func(ytdlp.Event) error) (ytdlp.Result, error) {
	if downloader.calls.Add(1) == 1 {
		return ytdlp.Result{}, &ytdlp.Error{Code: "network", Message: "temporary network failure"}
	}
	return ytdlp.Result{OutputPath: "verified-by-fake"}, nil
}

func TestQueueRetriesTransientFailure(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job, err := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.AudioM4A)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	fake := &retryDownloader{}
	queue := New(store, fake, 1)
	queue.retryDelay = 20 * time.Millisecond
	queue.Start(ctx)
	defer queue.Stop()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := store.Get(ctx, job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State == domain.Completed {
			if current.Attempt != 2 || fake.calls.Load() != 2 {
				t.Fatalf("retry result = %+v, calls %d", current, fake.calls.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("transient failure was not retried")
}

func (downloader waitingDownloader) Download(ctx context.Context, job domain.Job, _ func(ytdlp.Event) error) (ytdlp.Result, error) {
	downloader.started <- job.ID
	<-ctx.Done()
	downloader.cancelled <- job.ID
	return ytdlp.Result{}, ctx.Err()
}

func TestQueueClaimsAndCancelsActiveJob(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job, err := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.VideoBest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	fake := waitingDownloader{started: make(chan string, 1), cancelled: make(chan string, 1)}
	queue := New(store, fake, 2)
	queue.Start(ctx)
	defer queue.Stop()
	queue.Wake()
	select {
	case id := <-fake.started:
		if id != job.ID {
			t.Fatalf("started job = %s, want %s", id, job.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queued job was not started")
	}
	active, err := store.Get(ctx, job.ID)
	if err != nil || active.State != domain.Downloading {
		t.Fatalf("active job = %+v, %v", active, err)
	}
	previous := active.State
	if err := active.Transition(domain.Cancelled); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, active, previous); err != nil {
		t.Fatal(err)
	}
	queue.Cancel(job.ID)
	select {
	case <-fake.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("active downloader was not cancelled")
	}
	final, err := store.Get(ctx, job.ID)
	if err != nil || final.State != domain.Cancelled {
		t.Fatalf("final job = %+v, %v", final, err)
	}
}

func TestPausedJobIsNotFailedAndResumes(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job, _ := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.VideoBest)
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	fake := waitingDownloader{started: make(chan string, 2), cancelled: make(chan string, 2)}
	queue := New(store, fake, 2)
	queue.Start(ctx)
	defer queue.Stop()
	queue.Wake()
	select {
	case <-fake.started:
	case <-time.After(3 * time.Second):
		t.Fatal("job was not started")
	}
	running, _ := store.Get(ctx, job.ID)
	if err := running.Transition(domain.Paused); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, running, domain.Downloading); err != nil {
		t.Fatal(err)
	}
	queue.Cancel(job.ID)
	<-fake.cancelled
	time.Sleep(100 * time.Millisecond)
	if paused, _ := store.Get(ctx, job.ID); paused.State != domain.Paused {
		t.Fatalf("after pausing, state = %s", paused.State)
	}
	resumed, _ := store.Get(ctx, job.ID)
	if err := resumed.Transition(domain.Queued); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, resumed, domain.Paused); err != nil {
		t.Fatal(err)
	}
	queue.Wake()
	select {
	case <-fake.started:
	case <-time.After(3 * time.Second):
		t.Fatal("resumed job was not started again")
	}
}
