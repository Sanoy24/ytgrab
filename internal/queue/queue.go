package queue

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Sanoy24/ytgrab/internal/cooldown"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

type Store interface {
	Queued(context.Context, int) ([]domain.Job, error)
	Get(context.Context, string) (domain.Job, error)
	Update(context.Context, domain.Job, domain.State) error
}

type Downloader interface {
	Download(context.Context, domain.Job, func(ytdlp.Event) error) (ytdlp.Result, error)
}

type Queue struct {
	store      Store
	downloader Downloader
	workers    int
	wake       chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
	group      sync.WaitGroup
	mu         sync.Mutex
	active     map[string]context.CancelFunc
	retryDelay time.Duration

	// cooldown pauses all work while YouTube is limiting this network; nil disables it.
	cooldown *cooldown.Gate
	// spacing returns the minimum gap between starting two downloads.
	spacing   func() time.Duration
	nextStart time.Time
}

// maxBlockedAttempts is how many times a job is tried while YouTube keeps blocking.
const maxBlockedAttempts = 3

// jitteredSpacing spreads job starts 3–8 seconds apart so bursts look less automated.
func jitteredSpacing() time.Duration {
	return 3*time.Second + time.Duration(rand.Int64N(int64(5*time.Second)))
}

func New(store Store, downloader Downloader, workers int) *Queue {
	if workers < 1 {
		workers = 2
	}
	if workers > 4 {
		workers = 4
	}
	return &Queue{store: store, downloader: downloader, workers: workers, wake: make(chan struct{}, 1), active: make(map[string]context.CancelFunc), retryDelay: 2 * time.Second, spacing: jitteredSpacing}
}

// SetCooldown shares a pause gate with format inspection. Call before Start.
func (queue *Queue) SetCooldown(gate *cooldown.Gate) {
	queue.cooldown = gate
}

// Running returns how many downloads are in progress.
func (queue *Queue) Running() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return len(queue.active)
}

// Cooldown returns the shared pause gate, or nil.
func (queue *Queue) Cooldown() *cooldown.Gate {
	return queue.cooldown
}

// PausedUntil reports when downloads resume after YouTube limited this network.
func (queue *Queue) PausedUntil() (time.Time, bool) {
	if queue.cooldown == nil {
		return time.Time{}, false
	}
	return queue.cooldown.Until()
}

// Resume ends a pause early.
func (queue *Queue) Resume() {
	if queue.cooldown != nil {
		queue.cooldown.Resume()
	}
	queue.Wake()
}

func (queue *Queue) Start(parent context.Context) {
	queue.ctx, queue.cancel = context.WithCancel(parent)
	for range queue.workers {
		queue.group.Add(1)
		go queue.worker()
	}
	queue.Wake()
}

func (queue *Queue) Stop() {
	if queue.cancel != nil {
		queue.cancel()
	}
	queue.group.Wait()
}

func (queue *Queue) Wake() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}

// Cancel stops an active process after the API has durably marked it cancelled.
func (queue *Queue) Cancel(id string) {
	queue.mu.Lock()
	cancel := queue.active[id]
	queue.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (queue *Queue) worker() {
	defer queue.group.Done()
	timer := time.NewTicker(1 * time.Second)
	defer timer.Stop()
	for {
		if queue.ctx.Err() != nil {
			return
		}
		if queue.cooldown != nil && queue.cooldown.Wait(queue.ctx) != nil {
			return
		}
		jobs, err := queue.store.Queued(queue.ctx, 1)
		if err == nil && len(jobs) != 0 {
			if !queue.pace() {
				return
			}
			// Another worker may have been blocked while this one waited its turn.
			if _, paused := queue.PausedUntil(); paused {
				continue
			}
			queue.runJob(jobs[0])
			continue
		}
		select {
		case <-queue.ctx.Done():
			return
		case <-queue.wake:
		case <-timer.C:
		}
	}
}

// pace waits so consecutive downloads don't start at the same moment. It reports false
// when the queue is stopping.
func (queue *Queue) pace() bool {
	if queue.cooldown == nil {
		return true
	}
	queue.mu.Lock()
	now := time.Now()
	start := queue.nextStart
	if start.Before(now) {
		start = now
	}
	queue.nextStart = start.Add(queue.spacing())
	queue.mu.Unlock()
	wait := time.Until(start)
	if wait <= 0 {
		return true
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-queue.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (queue *Queue) runJob(job domain.Job) {
	previous := job.State
	if err := job.Transition(domain.Downloading); err != nil {
		return
	}
	if err := queue.store.Update(queue.ctx, job, previous); err != nil {
		return // Another worker claimed or cancelled this job.
	}
	runCtx, cancel := context.WithCancel(queue.ctx)
	queue.mu.Lock()
	queue.active[job.ID] = cancel
	queue.mu.Unlock()
	defer func() {
		queue.mu.Lock()
		delete(queue.active, job.ID)
		queue.mu.Unlock()
		cancel()
	}()
	current, err := queue.store.Get(queue.ctx, job.ID)
	if err != nil || current.State != domain.Downloading {
		return // Cancellation raced with worker startup.
	}

	lastPersisted := time.Time{}
	result, downloadErr := queue.downloader.Download(runCtx, job, func(event ytdlp.Event) error {
		oldState := job.State
		changed := false
		if event.Title != "" {
			job.Title = &event.Title
			changed = true
		}
		if event.Progress != nil {
			job.Progress = event.Progress
		}
		if event.State != "" && event.State != job.State {
			if err := job.Transition(event.State); err != nil {
				return err
			}
			changed = true
		}
		if !changed && time.Since(lastPersisted) < time.Second {
			return nil
		}
		job.UpdatedAt = time.Now().UTC()
		if err := queue.store.Update(runCtx, job, oldState); err != nil {
			return err
		}
		lastPersisted = time.Now()
		return nil
	})
	if queue.ctx.Err() != nil {
		return // Startup recovery will mark this interrupted on next launch.
	}
	current, err = queue.store.Get(queue.ctx, job.ID)
	if err != nil || current.State == domain.Cancelled {
		return
	}
	previous = current.State
	if downloadErr != nil {
		if errors.Is(downloadErr, sqlitestore.ErrConflict) {
			return
		}
		failure := &domain.JobError{Code: "download_failed", Message: "The download failed. Retry after checking the URL and tools."}
		var downloadError *ytdlp.Error
		if errors.As(downloadErr, &downloadError) {
			failure = &domain.JobError{Code: downloadError.Code, Message: downloadError.Message}
		}
		if failure.Code == "blocked" && queue.cooldown != nil {
			// Pause first so no worker picks the job up early. Until the attempts run out,
			// the job goes straight back to the queue and never appears failed.
			queue.cooldown.Block()
			if current.Attempt < maxBlockedAttempts && current.Requeue() == nil {
				_ = queue.store.Update(queue.ctx, current, previous)
				return
			}
		}
		current.Error = failure
		if err := current.Transition(domain.Failed); err != nil {
			return
		}
		if err := queue.store.Update(queue.ctx, current, previous); err == nil && failure.Code == "network" && current.Attempt < 3 {
			queue.scheduleRetry(current.ID, current.Attempt)
		}
		return
	}
	if queue.cooldown != nil {
		queue.cooldown.Success()
	}
	if result.Title != "" {
		current.Title = &result.Title
	}
	current.OutputPath = &result.OutputPath
	if err := current.Transition(domain.Completed); err == nil {
		_ = queue.store.Update(queue.ctx, current, previous)
	}
}

func (queue *Queue) scheduleRetry(id string, attempt int) {
	delay := queue.retryDelay * time.Duration(attempt)
	time.AfterFunc(delay, func() {
		if queue.ctx.Err() != nil {
			return
		}
		job, err := queue.store.Get(queue.ctx, id)
		if err != nil || job.State != domain.Failed || job.Attempt != attempt || job.Error == nil || job.Error.Code != "network" {
			return
		}
		if err := job.Transition(domain.Queued); err != nil {
			return
		}
		if err := queue.store.Update(queue.ctx, job, domain.Failed); err == nil {
			queue.Wake()
		}
	})
}
