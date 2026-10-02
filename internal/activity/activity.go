// Package activity summarizes the download queue for the tray: how many downloads are
// running or waiting, and which ones just finished or failed.
package activity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

// Summary is one report. Finished and Failed hold only downloads that ended since the
// previous report, and stay empty while a YTGrab page is open, since the page shows
// those itself.
type Summary struct {
	Downloading int
	Queued      int
	Finished    []string // titles
	Failed      []Failure
}

// Failure is a download that failed and won't be retried automatically.
type Failure struct {
	Title   string
	Message string
}

// Tooltip describes the queue in a few words, for the tray icon.
func (s Summary) Tooltip() string {
	var parts []string
	if s.Downloading > 0 {
		parts = append(parts, fmt.Sprintf("%d downloading", s.Downloading))
	}
	if s.Queued > 0 {
		parts = append(parts, fmt.Sprintf("%d queued", s.Queued))
	}
	if len(parts) == 0 {
		return "YTGrab is running"
	}
	return "YTGrab · " + strings.Join(parts, " · ")
}

// Watch polls list every interval until ctx ends and calls report when the counts change
// or downloads end. The first poll only records what already exists, so past downloads
// are never announced. pageOpen reports whether a YTGrab page is watching the queue.
func Watch(ctx context.Context, interval time.Duration, list func(context.Context) ([]domain.Job, error), pageOpen func() bool, report func(Summary)) {
	var seen map[string]domain.State
	var last Summary
	poll := func() {
		jobs, err := list(ctx)
		if err != nil {
			return
		}
		var summary Summary
		quiet := seen == nil || pageOpen()
		next := make(map[string]domain.State, len(jobs))
		for _, job := range jobs {
			next[job.ID] = job.State
			switch job.State {
			case domain.Queued:
				summary.Queued++
			case domain.Inspecting, domain.Downloading, domain.Processing:
				summary.Downloading++
			}
			if quiet {
				continue
			}
			if before, known := seen[job.ID]; known && !before.Active() {
				continue // ended before the last poll
			}
			switch job.State {
			case domain.Completed:
				summary.Finished = append(summary.Finished, title(job))
			case domain.Failed:
				if !retrying(job) {
					message := ""
					if job.Error != nil {
						message = job.Error.Message
					}
					summary.Failed = append(summary.Failed, Failure{Title: title(job), Message: message})
				}
			}
		}
		changed := seen == nil || summary.Downloading != last.Downloading || summary.Queued != last.Queued
		seen = next
		last = summary
		if changed || len(summary.Finished) > 0 || len(summary.Failed) > 0 {
			report(summary)
		}
	}
	poll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

// retrying matches the queue, which retries network failures by itself up to three times.
func retrying(job domain.Job) bool {
	return job.Error != nil && job.Error.Code == "network" && job.Attempt < 3
}

func title(job domain.Job) string {
	if job.Title != nil && *job.Title != "" {
		return *job.Title
	}
	return job.URL
}
