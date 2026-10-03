package queue

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func TestDownloadsWaitForTheWindow(t *testing.T) {
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
	// A window that opens two hours from now: closed for the whole test.
	h := time.Now().Hour()
	var mu sync.Mutex
	window := domain.Window{Start: (h + 2) % 24, End: (h + 3) % 24}
	fake := waitingDownloader{started: make(chan string, 1), cancelled: make(chan string, 1)}
	queue := New(store, fake, 2)
	queue.spacing = func() time.Duration { return 0 }
	queue.SetWindow(func() domain.Window { mu.Lock(); defer mu.Unlock(); return window })
	queue.Start(ctx)
	defer queue.Stop()
	if until, waiting := queue.WaitingUntil(); !waiting || until.Hour() != (h+2)%24 {
		t.Fatalf("WaitingUntil = %v, %v", until, waiting)
	}
	queue.Wake()
	select {
	case <-fake.started:
		t.Fatal("a download started outside the window")
	case <-time.After(500 * time.Millisecond):
	}
	mu.Lock()
	window = domain.Window{} // any time
	mu.Unlock()
	queue.Wake()
	select {
	case <-fake.started:
	case <-time.After(3 * time.Second):
		t.Fatal("the download didn't start once the window allowed it")
	}
}
