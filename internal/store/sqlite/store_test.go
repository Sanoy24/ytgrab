package sqlitestore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"ytgrab/internal/domain"
)

func TestStorePersistenceRecoveryAndRetry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.db")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	job, err := domain.NewJob("https://youtu.be/dQw4w9WgXcQ", domain.VideoBest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	duplicate, _ := domain.NewJob("https://www.youtube.com/watch?v=dQw4w9WgXcQ", domain.AudioM4A)
	if err := store.Create(ctx, duplicate); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate error = %v, want ErrDuplicate", err)
	}
	previous := job.State
	if err := job.Transition(domain.Downloading); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, job, previous); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	count, err := store.Recover(ctx)
	if err != nil || count != 1 {
		t.Fatalf("Recover = %d, %v", count, err)
	}
	restored, err := store.Get(ctx, job.ID)
	if err != nil || restored.State != domain.Failed || restored.Error == nil || restored.Error.Code != "interrupted" {
		t.Fatalf("restored job = %+v, %v", restored, err)
	}
	previous = restored.State
	if err := restored.Transition(domain.Queued); err != nil {
		t.Fatal(err)
	}
	if err := store.Update(ctx, restored, previous); err != nil {
		t.Fatal(err)
	}
	if restored.Attempt != 2 {
		t.Fatalf("retry attempt = %d, want 2", restored.Attempt)
	}
	queued, err := store.Queued(ctx, 10)
	if err != nil || len(queued) != 1 {
		t.Fatalf("queued jobs = %d, %v", len(queued), err)
	}
}
