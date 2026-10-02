package activity

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

type fakeQueue struct {
	mu   sync.Mutex
	jobs []domain.Job
}

func (q *fakeQueue) set(jobs ...domain.Job) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = jobs
}

func (q *fakeQueue) list(context.Context) ([]domain.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]domain.Job(nil), q.jobs...), nil
}

func job(id string, state domain.State) domain.Job {
	title := "Video " + id
	return domain.Job{ID: id, URL: "https://youtu.be/" + id, Title: &title, State: state}
}

func failed(id, code string, attempt int) domain.Job {
	j := job(id, domain.Failed)
	j.Attempt = attempt
	j.Error = &domain.JobError{Code: code, Message: "It broke."}
	return j
}

// run starts Watch and returns a function that waits for the next report.
func run(t *testing.T, q *fakeQueue, pageOpen *bool) func() Summary {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	reports := make(chan Summary, 10)
	var mu sync.Mutex
	open := func() bool { mu.Lock(); defer mu.Unlock(); return *pageOpen }
	go Watch(ctx, 10*time.Millisecond, q.list, open, func(s Summary) { reports <- s })
	return func() Summary {
		t.Helper()
		select {
		case s := <-reports:
			return s
		case <-time.After(2 * time.Second):
			t.Fatal("no report")
			return Summary{}
		}
	}
}

func TestReportsCountsAndNewlyEndedDownloads(t *testing.T) {
	q := &fakeQueue{}
	q.set(job("old", domain.Completed), job("a", domain.Downloading), job("b", domain.Queued), failed("c", "network", 1))
	pageOpen := false
	next := run(t, q, &pageOpen)

	first := next()
	if first.Downloading != 1 || first.Queued != 1 || len(first.Finished)+len(first.Failed) != 0 {
		t.Fatalf("first report = %+v", first)
	}
	if got := first.Tooltip(); got != "YTGrab · 1 downloading · 1 queued" {
		t.Fatalf("tooltip = %q", got)
	}

	// a finishes, b fails for good, c (a network failure) is still waiting to retry.
	q.set(job("old", domain.Completed), job("a", domain.Completed), failed("b", "private", 1), failed("c", "network", 1))
	ended := next()
	if len(ended.Finished) != 1 || ended.Finished[0] != "Video a" || len(ended.Failed) != 1 || ended.Failed[0].Title != "Video b" {
		t.Fatalf("ended = %+v", ended)
	}
	if ended.Tooltip() != "YTGrab is running" {
		t.Fatalf("idle tooltip = %q", ended.Tooltip())
	}

	// c is retried and finishes; a and b aren't announced again.
	q.set(job("a", domain.Completed), failed("b", "private", 1), job("c", domain.Queued))
	if s := next(); s.Queued != 1 || len(s.Finished)+len(s.Failed) != 0 {
		t.Fatalf("requeued = %+v", s)
	}
	q.set(job("a", domain.Completed), failed("b", "private", 1), job("c", domain.Completed))
	if s := next(); len(s.Finished) != 1 || s.Finished[0] != "Video c" {
		t.Fatalf("retried = %+v", s)
	}
}

func TestQuietWhileAPageIsOpen(t *testing.T) {
	q := &fakeQueue{}
	q.set(job("a", domain.Downloading))
	pageOpen := true
	next := run(t, q, &pageOpen)
	next()
	q.set(job("a", domain.Completed))
	if s := next(); s.Downloading != 0 || len(s.Finished) != 0 {
		t.Fatalf("with a page open = %+v", s)
	}
}
