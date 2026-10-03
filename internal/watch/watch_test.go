package watch

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// fakeLister returns whatever the channel or playlist holds right now.
type fakeLister struct {
	title string
	ids   []string // in listing order
	err   error
	calls int
}

func (f *fakeLister) ListLatest(_ context.Context, _, _ string, limit int) (ytdlp.Listing, error) {
	f.calls++
	if f.err != nil {
		return ytdlp.Listing{}, f.err
	}
	listing := ytdlp.Listing{Title: f.title}
	for i, id := range f.ids {
		if i == limit {
			break
		}
		listing.Entries = append(listing.Entries, ytdlp.PlaylistEntry{VideoID: id, Title: "Video " + id[:3]})
	}
	return listing, nil
}

func setup(t *testing.T) (*Service, *sqlitestore.Store, *fakeLister) {
	t.Helper()
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	lister := &fakeLister{title: "Google for Developers"}
	return New(store, lister, nil, nil), store, lister
}

// id pads a short name into a valid 11-character video ID.
func id(n string) string { return n + strings.Repeat("a", 11-len(n)) }

func queuedTitles(t *testing.T, store *sqlitestore.Store) []string {
	t.Helper()
	jobs, err := store.List(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i := len(jobs) - 1; i >= 0; i-- { // List is newest first
		out = append(out, *jobs[i].Title)
	}
	return out
}

func TestAddCountsExistingVideosAsSeen(t *testing.T) {
	service, store, lister := setup(t)
	ctx := context.Background()
	lister.ids = []string{id("new"), id("mid"), id("old")} // a channel lists its newest first
	watch, err := service.Add(ctx, "https://www.youtube.com/@GoogleDevelopers", domain.AudioM4A, true, 2)
	if err != nil {
		t.Fatal(err)
	}
	if watch.Title != "Google for Developers" || watch.LastNew != 2 || watch.Kind != "channel" {
		t.Fatalf("watch = %+v", watch)
	}
	// The two newest were queued, oldest first, into the channel's folder.
	if got := queuedTitles(t, store); len(got) != 2 || got[0] != "Video mid" || got[1] != "Video new" {
		t.Fatalf("queued = %v", got)
	}
	jobs, _ := store.List(ctx, 10)
	if jobs[0].Folder != "Google for Developers" {
		t.Fatalf("folder = %q", jobs[0].Folder)
	}
	// Nothing is new until the channel uploads something.
	if got, _ := service.Check(ctx, watch.ID); got.LastNew != 0 {
		t.Fatalf("check without uploads queued %d", got.LastNew)
	}
	lister.ids = append([]string{id("up2"), id("up1")}, lister.ids...)
	got, err := service.Check(ctx, watch.ID)
	if err != nil || got.LastNew != 2 || got.Downloaded != 4 {
		t.Fatalf("after uploads = %+v, %v", got, err)
	}
	if titles := queuedTitles(t, store); titles[2] != "Video up1" || titles[3] != "Video up2" {
		t.Fatalf("new uploads not queued oldest first: %v", titles)
	}
	if _, err := service.Add(ctx, "youtube.com/@GoogleDevelopers/videos", domain.Video720, false, 0); !errors.Is(err, sqlitestore.ErrWatchExists) {
		t.Fatalf("watching twice = %v", err)
	}
}

func TestCheckCapsNewVideosAndSkipsWhenBlocked(t *testing.T) {
	service, store, lister := setup(t)
	ctx := context.Background()
	watch, err := service.Add(ctx, "https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2", domain.AudioM4A, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxPerCheck+5; i++ { // a playlist grows at the end
		lister.ids = append(lister.ids, id(string(rune('a'+i%26))+string(rune('a'+i/26))))
	}
	got, _ := service.Check(ctx, watch.ID)
	if got.LastNew != MaxPerCheck {
		t.Fatalf("first check queued %d, want %d", got.LastNew, MaxPerCheck)
	}
	got, _ = service.Check(ctx, watch.ID)
	if got.LastNew != 5 {
		t.Fatalf("second check queued %d, want the remaining 5", got.LastNew)
	}
	before := *got.LastChecked
	lister.err = &ytdlp.Error{Code: "blocked", Message: "YouTube is limiting requests"}
	if got, err := service.Check(ctx, watch.ID); err == nil || got.LastChecked.After(before) || got.LastError != "" {
		t.Fatalf("a block was recorded against the watch: %+v, %v", got, err)
	}
	lister.err = &ytdlp.Error{Code: "video_unavailable", Message: "This channel or playlist is unavailable or private."}
	if got, _ := service.Check(ctx, watch.ID); got.LastError == "" {
		t.Fatal("a real failure wasn't shown on the watch")
	}
	if n, _ := store.List(ctx, 100); len(n) != MaxPerCheck+5 {
		t.Fatalf("%d jobs", len(n))
	}
}

func TestOnlyDueUnpausedWatchesAreChecked(t *testing.T) {
	service, _, lister := setup(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	a, _ := service.Add(ctx, "https://www.youtube.com/@GoogleDevelopers", domain.AudioM4A, false, 0)
	b, _ := service.Add(ctx, "https://www.youtube.com/@ChromeDevs", domain.AudioM4A, false, 0)
	paused := true
	if _, err := service.Update(ctx, b.ID, nil, nil, &paused); err != nil {
		t.Fatal(err)
	}
	lister.calls = 0
	service.checkDue(ctx)
	if lister.calls != 0 {
		t.Fatalf("checked %d watches before they were due", lister.calls)
	}
	now = now.Add(Interval)
	service.checkDue(ctx)
	if lister.calls != 1 {
		t.Fatalf("checked %d watches, want only the unpaused one", lister.calls)
	}
	_ = a
}

func TestBackfillIsLimited(t *testing.T) {
	service, _, _ := setup(t)
	if _, err := service.Add(context.Background(), "https://www.youtube.com/@GoogleDevelopers", domain.AudioM4A, false, MaxBackfill+1); !errors.Is(err, ErrInvalidBackfill) {
		t.Fatalf("backfill over the limit = %v", err)
	}
}
