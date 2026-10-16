package watch

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/library"
)

// pruneStore lists and removes downloads; the SQLite store does both.
type pruneStore interface {
	List(context.Context, int) ([]domain.Job, error)
	Delete(context.Context, string) error
}

// pruneAll applies every watch's keep limits.
func (s *Service) pruneAll(ctx context.Context) {
	watches, err := s.store.Watches(ctx)
	if err != nil {
		return
	}
	for _, watch := range watches {
		if ctx.Err() != nil {
			return
		}
		s.prune(ctx, watch)
	}
}

// prune deletes a watch's own finished downloads beyond its newest KeepLatest or older
// than KeepDays: the file, what was saved beside it, and the Library entry. Only downloads
// the watch queued are touched, never one added by hand. The videos stay seen, so the watch
// doesn't download them again.
func (s *Service) prune(ctx context.Context, watch domain.Watch) int {
	if watch.KeepLatest == 0 && watch.KeepDays == 0 {
		return 0
	}
	store, ok := s.store.(pruneStore)
	if !ok {
		return 0
	}
	s.pruning.Lock()
	defer s.pruning.Unlock()
	jobs, err := store.List(ctx, 500)
	if err != nil {
		return 0
	}
	var own []domain.Job
	for _, job := range jobs {
		if job.WatchID == watch.ID && job.State == domain.Completed {
			own = append(own, job)
		}
	}
	sort.SliceStable(own, func(i, j int) bool { return own[i].UpdatedAt.After(own[j].UpdatedAt) })
	cutoff := s.now().Add(-time.Duration(watch.KeepDays) * 24 * time.Hour)
	removed := 0
	for i, job := range own {
		beyond := watch.KeepLatest > 0 && i >= watch.KeepLatest
		expired := watch.KeepDays > 0 && job.UpdatedAt.Before(cutoff)
		if !beyond && !expired {
			continue
		}
		if path, owned := library.Owned(job); owned {
			if _, err := library.Remove(job, path); err != nil {
				continue // open in a player, say: tried again next round
			}
		}
		if err := store.Delete(ctx, job.ID); err == nil {
			removed++
		}
	}
	if removed > 0 {
		_, _ = fmt.Fprintf(s.output, "Watching %s: removed %d older video(s) to keep within its limit.\n", watch.Title, removed)
	}
	return removed
}
