package sqlitestore

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestWatches(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	watch, _ := domain.NewWatch("https://www.youtube.com/@GoogleDevelopers", domain.AudioM4A)
	if err := store.CreateWatch(ctx, watch, []string{"aaaaaaaaaaa", "bbbbbbbbbbb"}); err != nil {
		t.Fatal(err)
	}
	again, _ := domain.NewWatch("youtube.com/@GoogleDevelopers/videos", domain.Video720)
	if err := store.CreateWatch(ctx, again, nil); !errors.Is(err, ErrWatchExists) {
		t.Fatalf("second watch of the same channel = %v", err)
	}
	unseen, err := store.Unseen(ctx, watch.ID, []string{"ccccccccccc", "aaaaaaaaaaa", "ddddddddddd"})
	if err != nil || !reflect.DeepEqual(unseen, []string{"ccccccccccc", "ddddddddddd"}) {
		t.Fatalf("unseen = %v, %v", unseen, err)
	}
	if err := store.MarkSeen(ctx, watch.ID, []string{"ccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	if unseen, _ := store.Unseen(ctx, watch.ID, []string{"ccccccccccc", "ddddddddddd"}); !reflect.DeepEqual(unseen, []string{"ddddddddddd"}) {
		t.Fatalf("after MarkSeen = %v", unseen)
	}
	watch.Title, watch.Paused = "Google for Developers", true
	if err := store.UpdateWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	if list, _ := store.Watches(ctx); len(list) != 1 || list[0].Title != "Google for Developers" || !list[0].Paused {
		t.Fatalf("watches = %+v", list)
	}
	if err := store.DeleteWatch(ctx, watch.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := store.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM watch_seen").Scan(&left); err != nil || left != 0 {
		t.Fatalf("seen videos left behind: %d, %v", left, err)
	}
	if _, err := store.GetWatch(ctx, watch.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted watch = %v", err)
	}
}
