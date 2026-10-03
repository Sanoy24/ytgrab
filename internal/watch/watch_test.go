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
	ids   []string                       // in listing order
	extra map[string]ytdlp.PlaylistEntry // optional title, duration, or live status per ID
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
		entry := ytdlp.PlaylistEntry{VideoID: id, Title: "Video " + id[:3]}
		if e, ok := f.extra[id]; ok {
			entry.Title, entry.DurationSeconds, entry.LiveStatus = e.Title, e.DurationSeconds, e.LiveStatus
		}
		listing.Entries = append(listing.Entries, entry)
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
	watch, err := service.Add(ctx, "https://www.youtube.com/@GoogleDevelopers", Options{Preset: domain.AudioM4A, Folder: true, Backfill: 2})
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
	if _, err := service.Add(ctx, "youtube.com/@GoogleDevelopers/videos", Options{Preset: domain.Video720}); !errors.Is(err, sqlitestore.ErrWatchExists) {
		t.Fatalf("watching twice = %v", err)
	}
}

func TestCheckCapsNewVideosAndSkipsWhenBlocked(t *testing.T) {
	service, store, lister := setup(t)
	ctx := context.Background()
	watch, err := service.Add(ctx, "https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2", Options{Preset: domain.AudioM4A})
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
	a, _ := service.Add(ctx, "https://www.youtube.com/@GoogleDevelopers", Options{Preset: domain.AudioM4A})
	b, _ := service.Add(ctx, "https://www.youtube.com/@ChromeDevs", Options{Preset: domain.AudioM4A})
	paused := true
	if _, err := service.Update(ctx, b.ID, Changes{Paused: &paused}); err != nil {
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
	if _, err := service.Add(context.Background(), "https://www.youtube.com/@GoogleDevelopers", Options{Preset: domain.AudioM4A, Backfill: MaxBackfill + 1}); !errors.Is(err, ErrInvalidBackfill) {
		t.Fatalf("backfill over the limit = %v", err)
	}
}

func TestFiltersAndUpcomingVideos(t *testing.T) {
	service, store, lister := setup(t)
	ctx := context.Background()
	watch, err := service.Add(ctx, "https://www.youtube.com/@GoogleDevelopers", Options{Preset: domain.AudioM4A, MinMinutes: 2, Keywords: "gemma"})
	if err != nil {
		t.Fatal(err)
	}
	long, short := 600.0, 40.0
	lister.extra = map[string]ytdlp.PlaylistEntry{
		id("p1"): {Title: "Gemma premiere", DurationSeconds: &long, LiveStatus: "is_upcoming"},
		id("s1"): {Title: "Gemma in 40 seconds", DurationSeconds: &short},
		id("o1"): {Title: "Chrome news", DurationSeconds: &long},
		id("g1"): {Title: "Gemma 4 deep dive", DurationSeconds: &long},
	}
	lister.ids = []string{id("p1"), id("s1"), id("o1"), id("g1")}
	got, err := service.Check(ctx, watch.ID)
	if err != nil || got.LastNew != 1 {
		t.Fatalf("check = %+v, %v", got, err)
	}
	if titles := queuedTitles(t, store); len(titles) != 1 || titles[0] != "Gemma 4 deep dive" {
		t.Fatalf("queued = %v", titles)
	}
	// The premiere becomes available: now it downloads. The skipped ones never do.
	lister.extra[id("p1")] = ytdlp.PlaylistEntry{Title: "Gemma premiere", DurationSeconds: &long}
	if got, _ := service.Check(ctx, watch.ID); got.LastNew != 1 {
		t.Fatalf("after the premiere = %+v", got)
	}
	five := 5
	if _, err := service.Update(ctx, watch.ID, Changes{MinMinutes: &five}); err != nil {
		t.Fatal(err)
	}
	bad := 7
	if _, err := service.Update(ctx, watch.ID, Changes{IntervalHours: &bad}); !errors.Is(err, domain.ErrInvalidWatchOptions) {
		t.Fatalf("interval 7 accepted: %v", err)
	}
}

func TestEachWatchHasItsOwnInterval(t *testing.T) {
	service, _, lister := setup(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if _, err := service.Add(ctx, "https://www.youtube.com/@GoogleDevelopers", Options{Preset: domain.AudioM4A, IntervalHours: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(ctx, "https://www.youtube.com/@ChromeDevs", Options{Preset: domain.AudioM4A, IntervalHours: 24}); err != nil {
		t.Fatal(err)
	}
	lister.calls = 0
	now = now.Add(time.Hour)
	service.checkDue(ctx)
	if lister.calls != 1 {
		t.Fatalf("after an hour, %d checks; want only the hourly watch", lister.calls)
	}
}
