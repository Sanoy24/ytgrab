package sqlitestore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/domain"
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

func TestDeleteRemovesOnlyFinishedJobs(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	add := func(url string, state domain.State) domain.Job {
		t.Helper()
		job, err := domain.NewJob(url, domain.AudioM4A)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Create(ctx, job); err != nil {
			t.Fatal(err)
		}
		if state != domain.Queued {
			_ = job.Transition(domain.Downloading)
			if err := store.Update(ctx, job, domain.Queued); err != nil {
				t.Fatal(err)
			}
			if state != domain.Downloading {
				_ = job.Transition(state)
				if err := store.Update(ctx, job, domain.Downloading); err != nil {
					t.Fatal(err)
				}
			}
		}
		return job
	}
	done := add("https://youtu.be/dQw4w9WgXcQ", domain.Completed)
	failed := add("https://youtu.be/jNQXAC9IVRw", domain.Failed)
	running := add("https://youtu.be/aqz-KE-bpKQ", domain.Downloading)
	queued := add("https://youtu.be/M7lc1UVf-VE", domain.Queued)

	if err := store.Delete(ctx, running.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("deleting a running job = %v, want ErrConflict", err)
	}
	if err := store.Delete(ctx, "job_missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting a missing job = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, done.ID); err != nil {
		t.Fatal(err)
	}
	removed, err := store.DeleteFinished(ctx)
	if err != nil || removed != 1 {
		t.Fatalf("DeleteFinished = %d, %v; want 1 (the failed job)", removed, err)
	}
	jobs, _ := store.List(ctx, 10)
	if len(jobs) != 2 {
		t.Fatalf("remaining jobs = %d, want the running and queued ones", len(jobs))
	}
	for _, job := range jobs {
		if job.ID != running.ID && job.ID != queued.ID {
			t.Fatalf("unexpected remaining job %s (%s); failed was %s", job.ID, job.State, failed.ID)
		}
	}
}
