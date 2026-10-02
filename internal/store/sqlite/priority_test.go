package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestQueuedOrderAndPausedDuplicates(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var ids []string
	for _, url := range []string{"https://youtu.be/aaaaaaaaaaa", "https://youtu.be/bbbbbbbbbbb", "https://youtu.be/ccccccccccc"} {
		job, _ := domain.NewJob(url, domain.AudioM4A)
		if err := store.Create(ctx, job); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, job.ID)
		time.Sleep(2 * time.Millisecond)
	}
	// Move the newest to the top.
	last, _ := store.Get(ctx, ids[2])
	last.Priority = time.Now().UnixNano()
	if err := store.Update(ctx, last, domain.Queued); err != nil {
		t.Fatal(err)
	}
	queued, err := store.Queued(ctx, 10)
	if err != nil || len(queued) != 3 || queued[0].ID != ids[2] || queued[1].ID != ids[0] {
		t.Fatalf("order = %v, %v", jobIDs(queued), err)
	}
	// A paused download still blocks a second copy of the same video.
	first, _ := store.Get(ctx, ids[0])
	if err := first.Transition(domain.Paused); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, first, domain.Queued); err != nil {
		t.Fatal(err)
	}
	again, _ := domain.NewJob("https://youtu.be/aaaaaaaaaaa", domain.AudioM4A)
	if err := store.Create(ctx, again); err != ErrDuplicate {
		t.Fatalf("duplicate of a paused video = %v", err)
	}
	if queued, _ := store.Queued(ctx, 10); len(queued) != 2 {
		t.Fatalf("paused job is offered to workers: %v", jobIDs(queued))
	}
}

func jobIDs(jobs []domain.Job) []string {
	ids := make([]string, len(jobs))
	for i, job := range jobs {
		ids[i] = job.ID
	}
	return ids
}
