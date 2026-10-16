package watch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// finish completes a queued job with a file named the way YTGrab names them, at a time.
func finish(t *testing.T, store *sqlitestore.Store, job domain.Job, dir string, at time.Time) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(dir, "Video ["+job.VideoID+"].m4a")
	if err := os.WriteFile(path, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path[:len(path)-4]+".en.srt", []byte("subtitles"), 0o600)
	_ = job.Transition(domain.Downloading)
	if err := store.Update(ctx, job, domain.Queued); err != nil {
		t.Fatal(err)
	}
	job.OutputPath = &path
	_ = job.Transition(domain.Completed)
	job.UpdatedAt = at
	if err := store.Update(ctx, job, domain.Downloading); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKeepOnlyTheNewestDeletesOnlyTheWatchsOwnDownloads(t *testing.T) {
	service, store, lister := setup(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	lister.ids = []string{id("c3"), id("c2"), id("c1")}
	watch, err := service.Add(ctx, "https://www.youtube.com/@GoogleDevelopers", Options{Preset: domain.AudioM4A, Backfill: 3, IntervalHours: 6})
	if err != nil {
		t.Fatal(err)
	}
	// A video of the same channel added by hand: never the watch's to delete.
	manual, _ := domain.NewJob("https://youtu.be/"+id("hand"), domain.AudioM4A)
	if err := store.Create(ctx, manual); err != nil {
		t.Fatal(err)
	}
	jobs, _ := store.List(ctx, 50)
	dir := t.TempDir()
	files := map[string]string{}
	for i, job := range jobs {
		if job.ID != manual.ID && job.WatchID != watch.ID {
			t.Fatalf("%s wasn't linked to its watch", job.VideoID)
		}
		files[job.VideoID] = finish(t, store, job, dir, now.Add(-time.Duration(i+1)*48*time.Hour))
	}

	// Keep the newest one: the other two go, with their subtitles; the manual one stays.
	keep := 1
	if _, err := service.Update(ctx, watch.ID, Changes{KeepLatest: &keep}); err != nil {
		t.Fatal(err)
	}
	left, _ := store.List(ctx, 50)
	if len(left) != 2 {
		t.Fatalf("left %d downloads, want the newest of the watch and the manual one", len(left))
	}
	gone := 0
	for videoID, path := range files {
		_, err := os.Stat(path)
		if videoID == manual.VideoID && err != nil {
			t.Error("the manual download's file was deleted")
		}
		if os.IsNotExist(err) {
			gone++
			if _, err := os.Stat(path[:len(path)-4] + ".en.srt"); !os.IsNotExist(err) {
				t.Errorf("%s's subtitles were kept", videoID)
			}
		}
	}
	if gone != 2 {
		t.Errorf("%d files deleted, want 2", gone)
	}

	// Deleted videos stay seen: checking again doesn't download them back.
	if checked, err := service.Check(ctx, watch.ID); err != nil || checked.LastNew != 0 {
		t.Fatalf("check after pruning queued %d (%v)", checked.LastNew, err)
	}

	// Keep for a day instead: the newest is two days old, so it goes too.
	none, day := 0, 1
	if _, err := service.Update(ctx, watch.ID, Changes{KeepLatest: &none, KeepDays: &day}); err != nil {
		t.Fatal(err)
	}
	if left, _ := store.List(ctx, 50); len(left) != 1 || left[0].ID != manual.ID {
		t.Fatalf("after keep 1 day: %d left", len(left))
	}

	bad := -1
	if _, err := service.Update(ctx, watch.ID, Changes{KeepDays: &bad}); err == nil {
		t.Error("accepted a negative limit")
	}
}
