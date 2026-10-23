package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func completed(t *testing.T, store *sqlitestore.Store, link, title string) domain.Job {
	t.Helper()
	ctx := context.Background()
	job, _ := domain.NewJob(link, domain.AudioM4A)
	job.Title = &title
	_ = store.Create(ctx, job)
	_ = job.Transition(domain.Downloading)
	_ = store.Update(ctx, job, domain.Queued)
	path := filepath.Join(t.TempDir(), title+" ["+job.VideoID+"].m4a")
	_ = os.WriteFile(path, []byte("x"), 0o600)
	job.OutputPath = &path
	_ = job.Transition(domain.Completed)
	if err := store.Update(ctx, job, domain.Downloading); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestFindsVideosThatAreGone(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	completed(t, store, "https://youtu.be/jNQXAC9IVRw", "Me at the zoo")
	completed(t, store, "https://youtu.be/aqz-KE-bpKQ", "Deleted talk")
	completed(t, store, "https://youtu.be/dQw4w9WgXcQ", "Private video")
	completed(t, store, "https://vimeo.com/76979871", "Removed on Vimeo")

	answers := map[string]int{"jNQXAC9IVRw": 200, "aqz-KE-bpKQ": 404, "dQw4w9WgXcQ": 401, "76979871": 404}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for id, status := range answers {
			if strings.Contains(r.URL.RawQuery, id) {
				w.WriteHeader(status)
				return
			}
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	var reported []string
	checker := New(store, func() bool { return true }, func(titles []string) { reported = append(reported, titles...) })
	checker.now = func() time.Time { return now }
	checker.sleep = func(context.Context, time.Duration) bool { return true }
	checker.lookup = func(site, id string) string { return server.URL + "/oembed?site=" + site + "&id=" + id }

	checker.Round(ctx)
	if len(reported) != 2 {
		t.Fatalf("reported %v, want the deleted talk and the Vimeo video", reported)
	}
	gone, _ := store.GoneSince(ctx)
	if len(gone) != 2 || gone[":aqz-KE-bpKQ"].IsZero() || gone["vimeo:76979871"].IsZero() {
		t.Fatalf("gone = %v", gone)
	}
	// A private video (401) says nothing either way and is asked about again next round.
	due, _ := store.UpstreamDue(ctx, now.Add(-every), 50)
	if len(due) != 1 || due[0].VideoID != "dQw4w9WgXcQ" {
		t.Fatalf("still due = %+v", due)
	}

	// A week later the deleted talk is back online, and nothing is reported twice.
	now, reported = now.Add(8*24*time.Hour), nil
	answers["aqz-KE-bpKQ"] = 200
	checker.Round(ctx)
	if gone, _ := store.GoneSince(ctx); len(gone) != 1 || len(reported) != 0 {
		t.Errorf("after a week: gone %v, reported %v", gone, reported)
	}

	// The site asking to slow down ends the round straight away.
	now = now.Add(8 * 24 * time.Hour)
	for id := range answers {
		answers[id] = http.StatusTooManyRequests
	}
	checker.Round(ctx)
	if due, _ := store.UpstreamDue(ctx, now.Add(-every), 50); len(due) != 4 {
		t.Errorf("after 429: %d due, want all 4 untouched", len(due))
	}
}
