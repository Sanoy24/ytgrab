package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/settings"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// backupCopy is one YTGrab: its store, settings, and handler.
type backupCopy struct {
	store   *sqlitestore.Store
	prefs   *settings.Manager
	handler http.Handler
}

func newBackupCopy(t *testing.T) backupCopy {
	t.Helper()
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	prefs, err := settings.New(ctx, store, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	return backupCopy{store, prefs, NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil, prefs)}
}

func (c backupCopy) do(t *testing.T, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	c.handler.ServeHTTP(response, httptest.NewRequest(method, target, bytes.NewReader(body)))
	return response
}

func TestBackupRoundTrip(t *testing.T) {
	ctx := context.Background()
	old := newBackupCopy(t)
	folder := t.TempDir()
	if err := old.prefs.SetDownloadsDir(ctx, folder); err != nil {
		t.Fatal(err)
	}
	if err := old.prefs.SetMaxDownloads(ctx, 3); err != nil {
		t.Fatal(err)
	}
	watch, err := domain.NewWatch("https://vimeo.com/channels/staffpicks", domain.AudioM4A)
	if err != nil {
		t.Fatal(err)
	}
	watch.Title, watch.MinMinutes = "Vimeo Staff Picks", 5
	if err := old.store.CreateWatch(ctx, watch, []string{"1228694119", "1231743820"}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "Song [dQw4w9WgXcQ].m4a")
	if err := os.WriteFile(file, []byte("audio"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := finishedJob(t, old.store, "https://youtu.be/dQw4w9WgXcQ", domain.Completed, file)
	finishedJob(t, old.store, "https://youtu.be/jNQXAC9IVRw", domain.Failed, "")
	finishedJob(t, old.store, "https://youtu.be/aqz-KE-bpKQ", domain.Downloading, "") // the queue isn't backed up

	exported := old.do(t, http.MethodGet, "/api/backup", nil)
	if exported.Code != http.StatusOK || !strings.Contains(exported.Header().Get("Content-Disposition"), "ytgrab-backup-") {
		t.Fatalf("export = %d %v", exported.Code, exported.Header())
	}
	var backup Backup
	if err := json.Unmarshal(exported.Body.Bytes(), &backup); err != nil || len(backup.Watches) != 1 || len(backup.Watches[0].Seen) != 2 || len(backup.Jobs) != 2 || backup.Settings.MaxDownloads != 3 {
		t.Fatalf("backup = %s, %v", exported.Body.String(), err)
	}

	fresh := newBackupCopy(t)
	restored := fresh.do(t, http.MethodPost, "/api/backup/restore", exported.Body.Bytes())
	var result RestoreResult
	if restored.Code != http.StatusOK || json.Unmarshal(restored.Body.Bytes(), &result) != nil {
		t.Fatalf("restore = %d %s", restored.Code, restored.Body.String())
	}
	if result.WatchesAdded != 1 || result.JobsAdded != 2 || result.JobsInvalid != 0 || len(result.SettingsSkipped) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if fresh.prefs.MaxDownloads() != 3 || fresh.prefs.DownloadsDir() != folder {
		t.Errorf("settings not restored: %d %q", fresh.prefs.MaxDownloads(), fresh.prefs.DownloadsDir())
	}
	watches, _ := fresh.store.Watches(ctx)
	if len(watches) != 1 || watches[0].URL != watch.URL || watches[0].Title != "Vimeo Staff Picks" || watches[0].MinMinutes != 5 || watches[0].Site != domain.SiteVimeo {
		t.Fatalf("watches = %+v", watches)
	}
	// Videos seen before stay seen, so the restored watch doesn't download them again.
	if unseen, _ := fresh.store.Unseen(ctx, watches[0].ID, []string{"1228694119", "1231743820", "1231817291"}); len(unseen) != 1 || unseen[0] != "1231817291" {
		t.Errorf("unseen after restore = %v", unseen)
	}
	got, err := fresh.store.Get(ctx, done.ID)
	if err != nil || got.OutputPath == nil || *got.OutputPath != file {
		t.Errorf("finished download = %+v, %v", got, err)
	}

	// Restoring again changes nothing.
	again := fresh.do(t, http.MethodPost, "/api/backup/restore", exported.Body.Bytes())
	if json.Unmarshal(again.Body.Bytes(), &result) != nil || result.WatchesAdded != 0 || result.WatchesExisting != 1 || result.JobsAdded != 0 || result.JobsExisting != 2 {
		t.Fatalf("second restore = %s", again.Body.String())
	}
}

func TestRestoreChecksWhatItReads(t *testing.T) {
	fresh := newBackupCopy(t)
	if got := fresh.do(t, http.MethodPost, "/api/backup/restore", []byte(`{"jobs":[]}`)); got.Code != http.StatusBadRequest {
		t.Errorf("a file without the format = %d", got.Code)
	}
	if got := fresh.do(t, http.MethodPost, "/api/backup/restore", []byte(`{"format":"ytgrab-backup","version":9}`)); got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "newer YTGrab") {
		t.Errorf("a newer backup = %d %s", got.Code, got.Body.String())
	}
	backup := `{"format":"ytgrab-backup","version":1,"settings":{"downloads_dir":"Z:\\nowhere\\at\\all","max_downloads":2,"default_preset":"video-best"},
		"watches":[{"watch":{"url":"https://example.com/channel","preset":"audio-m4a"}}],
		"jobs":[
			{"id":"job_ok","url":"https://youtu.be/dQw4w9WgXcQ","video_id":"dQw4w9WgXcQ","state":"completed","preset":"audio-m4a",
			 "output_path":"C:\\Windows\\System32\\drivers\\etc\\hosts","created_at":"2026-10-01T10:00:00Z","updated_at":"2026-10-01T10:05:00Z"},
			{"id":"job_queued","url":"https://youtu.be/jNQXAC9IVRw","video_id":"jNQXAC9IVRw","state":"queued","preset":"audio-m4a","created_at":"2026-10-01T10:00:00Z","updated_at":"2026-10-01T10:00:00Z"},
			{"id":"job_badlink","url":"https://example.com/v","video_id":"x","state":"failed","preset":"audio-m4a","created_at":"2026-10-01T10:00:00Z","updated_at":"2026-10-01T10:00:00Z"}]}`
	restored := fresh.do(t, http.MethodPost, "/api/backup/restore", []byte(backup))
	var result RestoreResult
	if restored.Code != http.StatusOK || json.Unmarshal(restored.Body.Bytes(), &result) != nil {
		t.Fatalf("restore = %d %s", restored.Code, restored.Body.String())
	}
	if result.JobsAdded != 1 || result.JobsInvalid != 2 || result.WatchesAdded != 0 || len(result.SettingsSkipped) == 0 || result.SettingsSkipped[0] != "download folder" {
		t.Fatalf("result = %+v", result)
	}
	// A path that isn't the file YTGrab saved for this video is dropped, so "Delete file"
	// can never reach it.
	if job, err := fresh.store.Get(context.Background(), "job_ok"); err != nil || job.OutputPath != nil {
		t.Errorf("job = %+v, %v", job, err)
	}
}
