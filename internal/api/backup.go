package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/settings"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// backupFormat names YTGrab backup files; a file without it is refused.
const backupFormat = "ytgrab-backup"

// Backup is the file "Back up" saves: preferences, watches with the videos they've seen
// (so a restore doesn't download those again), and the Library.
type Backup struct {
	Format     string            `json:"format"`
	Version    int               `json:"version"`
	ExportedAt time.Time         `json:"exported_at"`
	AppVersion string            `json:"app_version,omitempty"`
	Settings   settings.Snapshot `json:"settings"`
	Watches    []BackupWatch     `json:"watches"`
	Jobs       []domain.Job      `json:"jobs"`
}

// BackupWatch is a watch and the videos it has seen.
type BackupWatch struct {
	Watch domain.Watch `json:"watch"`
	Seen  []string     `json:"seen"`
}

// RestoreResult says what a restore added, kept, and skipped.
type RestoreResult struct {
	WatchesAdded    int      `json:"watches_added"`
	WatchesExisting int      `json:"watches_existing"`
	JobsAdded       int      `json:"jobs_added"`
	JobsExisting    int      `json:"jobs_existing"`
	JobsInvalid     int      `json:"jobs_invalid"`
	SettingsSkipped []string `json:"settings_skipped"`
}

// backupStore is the job store's part a backup needs.
type backupStore interface {
	JobStore
	Watches(context.Context) ([]domain.Watch, error)
	CreateWatch(context.Context, domain.Watch, []string) error
	SeenIDs(context.Context, string) ([]string, error)
}

// backupSettings is the settings value's part a backup needs.
type backupSettings interface {
	Snapshot() settings.Snapshot
	Restore(context.Context, settings.Snapshot) []string
}

// Limits on what a restore reads.
const (
	maxBackupBytes  = 32 << 20
	maxSeenPerWatch = 20000
)

func addBackupRoutes(mux *http.ServeMux, store backupStore, prefs backupSettings, version string) {
	mux.HandleFunc("GET /api/backup", func(w http.ResponseWriter, r *http.Request) {
		backup := Backup{Format: backupFormat, Version: 1, ExportedAt: time.Now().UTC(), AppVersion: version, Settings: prefs.Snapshot(), Watches: []BackupWatch{}, Jobs: []domain.Job{}}
		watches, err := store.Watches(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not read the watches.")
			return
		}
		for _, watch := range watches {
			seen, err := store.SeenIDs(r.Context(), watch.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "storage", "Could not read the watches.")
				return
			}
			backup.Watches = append(backup.Watches, BackupWatch{Watch: watch, Seen: seen})
		}
		jobs, err := store.List(r.Context(), 500)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not read the Library.")
			return
		}
		for _, job := range jobs {
			if !job.State.Active() { // the queue belongs to this computer's session
				backup.Jobs = append(backup.Jobs, job)
			}
		}
		name := "ytgrab-backup-" + backup.ExportedAt.Format("2006-01-02") + ".json"
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		writeJSON(w, http.StatusOK, backup)
	})

	mux.HandleFunc("POST /api/backup/restore", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBackupBytes)
		var backup Backup
		if err := json.NewDecoder(r.Body).Decode(&backup); err != nil || backup.Format != backupFormat {
			writeError(w, http.StatusBadRequest, "invalid_backup", "This isn't a YTGrab backup file.")
			return
		}
		if backup.Version > 1 {
			writeError(w, http.StatusBadRequest, "invalid_backup", "This backup comes from a newer YTGrab. Update YTGrab, then restore it.")
			return
		}
		result, err := restore(r.Context(), store, prefs, backup)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not restore the backup: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

// restore merges a backup into this copy: settings go through their usual checks, and
// watches and Library entries already here are kept as they are.
func restore(ctx context.Context, store backupStore, prefs backupSettings, backup Backup) (RestoreResult, error) {
	result := RestoreResult{SettingsSkipped: prefs.Restore(ctx, backup.Settings)}
	if result.SettingsSkipped == nil {
		result.SettingsSkipped = []string{}
	}
	existing, err := store.Watches(ctx)
	if err != nil {
		return result, err
	}
	have := map[string]bool{}
	for _, watch := range existing {
		have[watch.URL] = true
	}
	for _, saved := range backup.Watches {
		watch, ok := restoredWatch(saved.Watch)
		if !ok {
			continue
		}
		if have[watch.URL] {
			result.WatchesExisting++
			continue
		}
		seen := []string{}
		for _, id := range saved.Seen {
			if len(seen) < maxSeenPerWatch && domain.ValidWatchVideoID(watch.Site, id) {
				seen = append(seen, id)
			}
		}
		if err := store.CreateWatch(ctx, watch, seen); err != nil {
			return result, fmt.Errorf("add a watch: %w", err)
		}
		have[watch.URL] = true
		result.WatchesAdded++
	}
	for _, saved := range backup.Jobs {
		job, ok := restoredJob(saved)
		if !ok {
			result.JobsInvalid++
			continue
		}
		if err := store.Create(ctx, job); errors.Is(err, sqlitestore.ErrDuplicate) {
			result.JobsExisting++
		} else if err != nil {
			return result, fmt.Errorf("add a Library entry: %w", err)
		} else {
			result.JobsAdded++
		}
	}
	return result, nil
}

// restoredWatch rebuilds a saved watch from its link, so only links YTGrab accepts come
// back, keeping its options where they are valid.
func restoredWatch(saved domain.Watch) (domain.Watch, bool) {
	watch, err := domain.NewWatch(saved.URL, saved.Preset)
	if err != nil {
		return domain.Watch{}, false
	}
	watch.Title = truncate(saved.Title, 300)
	watch.Folder, watch.Paused = saved.Folder, saved.Paused
	if domain.ValidWatchOptions(saved.MinMinutes, saved.Keywords, saved.IntervalHours) {
		watch.MinMinutes, watch.Keywords, watch.IntervalHours = saved.MinMinutes, saved.Keywords, saved.IntervalHours
	}
	if !saved.CreatedAt.IsZero() {
		watch.CreatedAt = saved.CreatedAt
	}
	watch.Downloaded = max(saved.Downloaded, 0)
	return watch, true
}

// restoredJob checks a saved Library entry: a finished, failed, or cancelled download of
// a link YTGrab accepts. Its file path is kept only when it is the file YTGrab saved for
// that video, so "Delete file" can never reach anything else.
func restoredJob(saved domain.Job) (domain.Job, bool) {
	if saved.State.Active() || (saved.State != domain.Completed && saved.State != domain.Failed && saved.State != domain.Cancelled) {
		return domain.Job{}, false
	}
	if !strings.HasPrefix(saved.ID, "job_") || len(saved.ID) > 64 {
		return domain.Job{}, false
	}
	url, videoID, err := domain.ParseVideoURL(saved.URL)
	if err != nil || videoID != saved.VideoID {
		return domain.Job{}, false
	}
	job := saved
	job.URL, job.Site = url, domain.SiteOf(url)
	job.Priority = 0
	job.Thumbnail = domain.SafeThumbnail(saved.Thumbnail)
	job.Folder = domain.SafeFolderName(saved.Folder)
	if job.Title != nil {
		title := truncate(*job.Title, 500)
		job.Title = &title
	}
	if job.Preset != nil && !job.Preset.Valid() {
		return domain.Job{}, false
	}
	if job.Format != nil && !job.Format.Valid() {
		return domain.Job{}, false
	}
	if job.OutputPath != nil {
		path := *job.OutputPath
		if job.State != domain.Completed || !filepath.IsAbs(path) || !strings.Contains(filepath.Base(path), "["+job.VideoID+"]") {
			job.OutputPath = nil
		}
	}
	if job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() {
		return domain.Job{}, false
	}
	return job, true
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return strings.ToValidUTF8(text[:limit], "")
}
