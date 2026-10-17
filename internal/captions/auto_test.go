package captions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestAutomaticCaptionsOnlyNewDownloadsWithoutSubtitles(t *testing.T) {
	dir := t.TempDir()
	oldModel := model
	t.Cleanup(func() { model = oldModel })
	model = download{"ggml-base.bin", "", 5}
	s := New(dir, nil, nil, nil)
	s.goos, s.goarch = "windows", "amd64"
	_ = os.MkdirAll(filepath.Join(s.dir, "whisper"), 0o755)
	_ = os.WriteFile(filepath.Join(s.dir, "whisper", "whisper-cli.exe"), []byte("cli"), 0o755)
	_ = os.WriteFile(s.modelPath(), []byte("model"), 0o644)

	since := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	media := t.TempDir()
	finished := func(id, videoID string, at time.Time, subtitles bool) domain.Job {
		path := filepath.Join(media, "Talk ["+videoID+"].m4a")
		_ = os.WriteFile(path, []byte("audio"), 0o600)
		if subtitles {
			_ = os.WriteFile(filepath.Join(media, "Talk ["+videoID+"].en.srt"), []byte("1\n"), 0o600)
		}
		return domain.Job{ID: id, VideoID: videoID, State: domain.Completed, OutputPath: &path, UpdatedAt: at}
	}
	jobs := []domain.Job{
		finished("old", "aaaaaaaaaaa", since.Add(-time.Hour), false),
		finished("subbed", "bbbbbbbbbbb", since.Add(time.Hour), true),
		finished("new", "ccccccccccc", since.Add(time.Hour), false),
	}
	byID := map[string]domain.Job{}
	for _, job := range jobs {
		byID[job.ID] = job
	}
	s.job = func(_ context.Context, id string) (domain.Job, error) {
		if job, ok := byID[id]; ok {
			return job, nil
		}
		return domain.Job{}, errors.New("not found")
	}
	on := false
	s.Automatic(func() (bool, time.Time) { return on, since }, func(context.Context) ([]domain.Job, error) { return jobs, nil })

	s.queueNew(context.Background())
	if len(s.Status().Jobs) != 0 {
		t.Fatalf("queued while off: %v", s.Status().Jobs)
	}
	on = true
	s.queueNew(context.Background())
	got := s.Status().Jobs
	if len(got) != 1 || got["new"].State != Queued {
		t.Fatalf("queued %v, want only the new download without subtitles", got)
	}
	// A download that failed isn't tried again and again.
	s.set("new", JobState{State: Failed, Error: "boom"})
	s.queueNew(context.Background())
	if s.Status().Jobs["new"].State != Failed {
		t.Error("a failed download was queued again")
	}
}

func TestDetectedLanguage(t *testing.T) {
	log := "whisper_init_from_file: loading model\nwhisper_full_with_state: auto-detected language: es (p = 0.979516)\n"
	if m := detectedLanguage.FindStringSubmatch(log); m == nil || m[1] != "es" {
		t.Errorf("detected = %v", m)
	}
	if detectedLanguage.MatchString("language: auto") {
		t.Error("matched a line without a detected language")
	}
}
