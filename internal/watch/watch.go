// Package watch follows YouTube channels and playlists and queues their new videos.
package watch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

const (
	// Interval is how often a watch is checked unless it chose otherwise.
	Interval = 6 * time.Hour
	// A check lists a channel's newest uploads, or a whole playlist up to a limit.
	channelLimit  = 30
	playlistLimit = 500
	// MaxPerCheck caps the videos one check queues; the rest wait for the next check.
	MaxPerCheck = 25
	// MaxBackfill is the most already-published videos a new watch may queue.
	MaxBackfill = 10
)

var ErrInvalidBackfill = fmt.Errorf("Choose to download between 0 and %d of the latest videos.", MaxBackfill)

type Store interface {
	CreateWatch(context.Context, domain.Watch, []string) error
	Watches(context.Context) ([]domain.Watch, error)
	GetWatch(context.Context, string) (domain.Watch, error)
	UpdateWatch(context.Context, domain.Watch) error
	DeleteWatch(context.Context, string) error
	Unseen(context.Context, string, []string) ([]string, error)
	MarkSeen(context.Context, string, []string) error
	Create(context.Context, domain.Job) error
}

type Lister interface {
	ListLatest(ctx context.Context, kind, url string, limit int) (ytdlp.Listing, error)
}

// Options are chosen when a watch is added.
type Options struct {
	Preset        domain.Preset
	Folder        bool
	Backfill      int // how many of the newest videos to download right away
	MinMinutes    int
	Keywords      string
	IntervalHours int
}

// Changes edit a watch; nil fields stay as they are.
type Changes struct {
	Preset        *domain.Preset
	Folder        *bool
	Paused        *bool
	MinMinutes    *int
	Keywords      *string
	IntervalHours *int
}

type Service struct {
	store  Store
	lister Lister
	wake   func() // starts queued downloads
	output io.Writer
	now    func() time.Time
	mu     sync.Mutex // one listing at a time
}

func New(store Store, lister Lister, wake func(), output io.Writer) *Service {
	if wake == nil {
		wake = func() {}
	}
	if output == nil {
		output = io.Discard
	}
	return &Service{store: store, lister: lister, wake: wake, output: output, now: time.Now}
}

func (s *Service) List(ctx context.Context) ([]domain.Watch, error) {
	return s.store.Watches(ctx)
}

// Add starts watching a channel or playlist. What it holds now counts as seen, except the
// newest backfill videos, which are queued right away.
func (s *Service) Add(ctx context.Context, rawURL string, opts Options) (domain.Watch, error) {
	if opts.Backfill < 0 || opts.Backfill > MaxBackfill {
		return domain.Watch{}, ErrInvalidBackfill
	}
	if !domain.ValidWatchOptions(opts.MinMinutes, opts.Keywords, opts.IntervalHours) {
		return domain.Watch{}, domain.ErrInvalidWatchOptions
	}
	watch, err := domain.NewWatch(rawURL, opts.Preset)
	if err != nil {
		return domain.Watch{}, err
	}
	watch.Folder, watch.MinMinutes, watch.Keywords, watch.IntervalHours = opts.Folder, opts.MinMinutes, opts.Keywords, opts.IntervalHours
	s.mu.Lock()
	defer s.mu.Unlock()
	listing, err := s.lister.ListLatest(ctx, watch.Kind, watch.URL, limitFor(watch.Kind))
	if err != nil {
		return domain.Watch{}, err
	}
	watch.Title = listing.Title
	if watch.Title == "" {
		watch.Title = watch.URL
	}
	now := s.now().UTC()
	watch.LastChecked = &now
	// Everything available now counts as seen; upcoming and live videos stay unseen so
	// they are picked up once they can be downloaded.
	available := availableEntries(listing.Entries)
	if err := s.store.CreateWatch(ctx, watch, videoIDs(available)); err != nil {
		return domain.Watch{}, err
	}
	var wanted []string
	for _, entry := range available {
		if watch.Wants(entry.Title, entry.DurationSeconds) {
			wanted = append(wanted, entry.VideoID)
		}
	}
	newest := oldestFirst(watch.Kind, wanted)
	if len(newest) > opts.Backfill {
		newest = newest[len(newest)-opts.Backfill:]
	}
	queued, err := s.queue(ctx, watch, newest, titles(listing.Entries))
	watch.LastNew, watch.Downloaded = queued, queued
	if updateErr := s.store.UpdateWatch(ctx, watch); err == nil {
		err = updateErr
	}
	return watch, err
}

// Check lists a watch now and queues the videos it hasn't seen, oldest first.
func (s *Service) Check(ctx context.Context, id string) (domain.Watch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	watch, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return domain.Watch{}, err
	}
	listing, err := s.lister.ListLatest(ctx, watch.Kind, watch.URL, limitFor(watch.Kind))
	if err != nil {
		if blocked(err) {
			return watch, err // not a problem with this watch; it is checked again soon
		}
		now := s.now().UTC()
		watch.LastChecked, watch.LastError, watch.LastNew = &now, err.Error(), 0
		_ = s.store.UpdateWatch(ctx, watch)
		return watch, err
	}
	available := availableEntries(listing.Entries)
	unseenIDs, err := s.store.Unseen(ctx, watch.ID, videoIDs(available))
	if err != nil {
		return watch, err
	}
	// New videos the filters skip are marked seen, so they are never downloaded later.
	entries := byID(available)
	var unseen, skipped []string
	for _, id := range unseenIDs {
		if entry := entries[id]; watch.Wants(entry.Title, entry.DurationSeconds) {
			unseen = append(unseen, id)
		} else {
			skipped = append(skipped, id)
		}
	}
	if err := s.store.MarkSeen(ctx, watch.ID, skipped); err != nil {
		return watch, err
	}
	unseen = oldestFirst(watch.Kind, unseen)
	if len(unseen) > MaxPerCheck {
		unseen = unseen[:MaxPerCheck]
	}
	queued, err := s.queue(ctx, watch, unseen, titles(listing.Entries))
	now := s.now().UTC()
	watch.LastChecked, watch.LastError, watch.LastNew = &now, "", queued
	watch.Downloaded += queued
	if listing.Title != "" {
		watch.Title = listing.Title
	}
	if updateErr := s.store.UpdateWatch(ctx, watch); err == nil {
		err = updateErr
	}
	if queued > 0 {
		_, _ = fmt.Fprintf(s.output, "Watching %s: queued %d new video(s).\n", watch.Title, queued)
	}
	return watch, err
}

// Update changes a watch's format, folder choice, filters, interval, or pause. Filters
// apply to videos found from then on.
func (s *Service) Update(ctx context.Context, id string, changes Changes) (domain.Watch, error) {
	watch, err := s.store.GetWatch(ctx, id)
	if err != nil {
		return domain.Watch{}, err
	}
	if changes.Preset != nil {
		if !changes.Preset.Valid() {
			return domain.Watch{}, domain.ErrInvalidPreset
		}
		watch.Preset = *changes.Preset
	}
	if changes.Folder != nil {
		watch.Folder = *changes.Folder
	}
	if changes.Paused != nil {
		watch.Paused = *changes.Paused
	}
	if changes.MinMinutes != nil {
		watch.MinMinutes = *changes.MinMinutes
	}
	if changes.Keywords != nil {
		watch.Keywords = *changes.Keywords
	}
	if changes.IntervalHours != nil {
		watch.IntervalHours = *changes.IntervalHours
	}
	if !domain.ValidWatchOptions(watch.MinMinutes, watch.Keywords, watch.IntervalHours) {
		return domain.Watch{}, domain.ErrInvalidWatchOptions
	}
	return watch, s.store.UpdateWatch(ctx, watch)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.store.DeleteWatch(ctx, id)
}

// Run checks due watches a few minutes after start and then every quarter hour, one at a
// time, until ctx ends. A round stops while YouTube is limiting the network.
func (s *Service) Run(ctx context.Context) {
	timer := time.NewTimer(3 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.checkDue(ctx)
		timer.Reset(15 * time.Minute)
	}
}

func (s *Service) checkDue(ctx context.Context) {
	watches, err := s.store.Watches(ctx)
	if err != nil {
		return
	}
	for _, watch := range watches {
		if ctx.Err() != nil || !s.due(watch) {
			continue
		}
		if _, err := s.Check(ctx, watch.ID); blocked(err) {
			return
		}
	}
}

func (s *Service) due(watch domain.Watch) bool {
	return !watch.Paused && (watch.LastChecked == nil || s.now().Sub(*watch.LastChecked) >= watch.Interval())
}

// queue creates a download for each video and marks it seen. A video already in the
// queue counts as handled.
func (s *Service) queue(ctx context.Context, watch domain.Watch, ids []string, titles map[string]string) (int, error) {
	queued := 0
	for _, id := range ids {
		job, err := domain.NewJob(domain.VideoURL(id), watch.Preset)
		if err != nil {
			return queued, err
		}
		if title := titles[id]; title != "" {
			job.Title = &title
		}
		if watch.Folder {
			job.Folder = domain.SafeFolderName(watch.Title)
		}
		switch err := s.store.Create(ctx, job); {
		case err == nil:
			queued++
		case errors.Is(err, sqlitestore.ErrDuplicate):
		default:
			return queued, err
		}
		if err := s.store.MarkSeen(ctx, watch.ID, []string{id}); err != nil {
			return queued, err
		}
	}
	if queued > 0 {
		s.wake()
	}
	return queued, nil
}

func blocked(err error) bool {
	var toolError *ytdlp.Error
	return errors.As(err, &toolError) && toolError.Code == "blocked"
}

func limitFor(kind string) int {
	if kind == "playlist" {
		return playlistLimit
	}
	return channelLimit
}

// oldestFirst orders listed IDs by age: a channel lists its newest upload first, a
// playlist in its own order (new videos usually at the end).
func oldestFirst(kind string, ids []string) []string {
	ordered := slices.Clone(ids)
	if kind == "channel" {
		slices.Reverse(ordered)
	}
	return ordered
}

// availableEntries drops premieres and live streams, which can't be downloaded yet.
func availableEntries(entries []ytdlp.PlaylistEntry) []ytdlp.PlaylistEntry {
	var available []ytdlp.PlaylistEntry
	for _, entry := range entries {
		switch entry.LiveStatus {
		case "is_upcoming", "is_live", "post_live":
		default:
			available = append(available, entry)
		}
	}
	return available
}

func byID(entries []ytdlp.PlaylistEntry) map[string]ytdlp.PlaylistEntry {
	m := make(map[string]ytdlp.PlaylistEntry, len(entries))
	for _, entry := range entries {
		m[entry.VideoID] = entry
	}
	return m
}

func videoIDs(entries []ytdlp.PlaylistEntry) []string {
	ids := make([]string, len(entries))
	for i, entry := range entries {
		ids[i] = entry.VideoID
	}
	return ids
}

func titles(entries []ytdlp.PlaylistEntry) map[string]string {
	titles := make(map[string]string, len(entries))
	for _, entry := range entries {
		titles[entry.VideoID] = entry.Title
	}
	return titles
}
