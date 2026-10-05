package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/reveal"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func finishedJob(t *testing.T, store *sqlitestore.Store, url string, state domain.State, output string) domain.Job {
	t.Helper()
	ctx := context.Background()
	job, err := domain.NewJob(url, domain.VideoBest)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	_ = job.Transition(domain.Downloading)
	if err := store.Update(ctx, job, domain.Queued); err != nil {
		t.Fatal(err)
	}
	if state == domain.Downloading {
		return job
	}
	if output != "" {
		job.OutputPath = &output
	}
	_ = job.Transition(state)
	if err := store.Update(ctx, job, domain.Downloading); err != nil {
		t.Fatal(err)
	}
	return job
}

func TestShowInFolderAndRemoveFromHistory(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var shown []string
	showFile = func(path string) error { shown = append(shown, path); return nil }
	defer func() { showFile = reveal.Show }()

	dir := t.TempDir()
	file := filepath.Join(dir, "Title [dQw4w9WgXcQ] 1080p.mp4")
	if err := os.WriteFile(file, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	done := finishedJob(t, store, "https://youtu.be/dQw4w9WgXcQ", domain.Completed, file)
	moved := finishedJob(t, store, "https://youtu.be/jNQXAC9IVRw", domain.Completed, filepath.Join(dir, "Gone [jNQXAC9IVRw] 720p.mp4"))
	failed := finishedJob(t, store, "https://youtu.be/aqz-KE-bpKQ", domain.Failed, "")
	running := finishedJob(t, store, "https://youtu.be/M7lc1UVf-VE", domain.Downloading, "")
	// A record whose file name doesn't carry its video ID must never delete that file.
	other := filepath.Join(dir, "important.docx")
	if err := os.WriteFile(other, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	odd := finishedJob(t, store, "https://youtu.be/9bZkp7q19f0", domain.Completed, other)

	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	do := func(method, target string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, nil))
		return response
	}
	if got := do(http.MethodPost, "/api/jobs/"+done.ID+"/reveal"); got.Code != http.StatusNoContent || len(shown) != 1 || shown[0] != file {
		t.Fatalf("reveal = %d %s, shown %v", got.Code, got.Body.String(), shown)
	}
	if got := do(http.MethodPost, "/api/jobs/"+moved.ID+"/reveal"); got.Code != http.StatusNotFound {
		t.Fatalf("reveal moved file = %d %s", got.Code, got.Body.String())
	}
	if got := do(http.MethodPost, "/api/jobs/"+failed.ID+"/reveal"); got.Code != http.StatusConflict {
		t.Fatalf("reveal failed job = %d %s", got.Code, got.Body.String())
	}
	if got := do(http.MethodDelete, "/api/jobs/"+running.ID); got.Code != http.StatusConflict {
		t.Fatalf("remove running job = %d %s", got.Code, got.Body.String())
	}
	if got := do(http.MethodDelete, "/api/jobs/"+odd.ID+"?delete_file=true"); got.Code != http.StatusBadRequest {
		t.Fatalf("delete unrecognized file = %d %s", got.Code, got.Body.String())
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("an unrelated file was deleted")
	}
	if got := do(http.MethodDelete, "/api/jobs/"+done.ID+"?delete_file=true"); got.Code != http.StatusNoContent {
		t.Fatalf("remove with file = %d %s", got.Code, got.Body.String())
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("downloaded file was not deleted")
	}
	got := do(http.MethodPost, "/api/history/clear")
	var body struct {
		Removed int `json:"removed"`
	}
	if got.Code != http.StatusOK || json.Unmarshal(got.Body.Bytes(), &body) != nil || body.Removed != 3 {
		t.Fatalf("clear = %d %s", got.Code, got.Body.String())
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("clearing history deleted a file")
	}
	jobs, _ := store.List(context.Background(), 10)
	if len(jobs) != 1 || jobs[0].ID != running.ID {
		t.Fatalf("remaining = %+v", jobs)
	}
}

func TestClearHistoryWithFiles(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	file := write("Title [dQw4w9WgXcQ] 1080p.mp4")
	other := write("important.docx")
	finishedJob(t, store, "https://youtu.be/dQw4w9WgXcQ", domain.Completed, file)
	finishedJob(t, store, "https://youtu.be/jNQXAC9IVRw", domain.Completed, filepath.Join(dir, "Gone [jNQXAC9IVRw] 720p.mp4"))
	finishedJob(t, store, "https://youtu.be/9bZkp7q19f0", domain.Completed, other)
	finishedJob(t, store, "https://youtu.be/aqz-KE-bpKQ", domain.Failed, "")
	running := finishedJob(t, store, "https://youtu.be/M7lc1UVf-VE", domain.Downloading, "")
	// A file that can't be deleted keeps its entry: a non-empty folder stands in for a
	// file open in another program.
	stuck := filepath.Join(dir, "Stuck [kJQP7kiw5Fk] 720p.mp4")
	if err := os.MkdirAll(filepath.Join(stuck, "inside"), 0o700); err != nil {
		t.Fatal(err)
	}
	stuckJob := finishedJob(t, store, "https://youtu.be/kJQP7kiw5Fk", domain.Completed, stuck)

	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/history/clear?delete_files=true", nil))
	var body clearResult
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("clear = %d %s", response.Code, response.Body.String())
	}
	if body != (clearResult{Removed: 4, FilesDeleted: 1, Kept: 1}) {
		t.Fatalf("result = %+v", body)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("the downloaded file was not deleted")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("a file whose name lacks the video ID was deleted")
	}
	jobs, _ := store.List(context.Background(), 10)
	if len(jobs) != 2 || !((jobs[0].ID == running.ID && jobs[1].ID == stuckJob.ID) || (jobs[1].ID == running.ID && jobs[0].ID == stuckJob.ID)) {
		t.Fatalf("remaining = %+v", jobs)
	}
}

func TestDeletingASplitDownloadRemovesItsChapters(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "Talk [dQw4w9WgXcQ].m4a")
	folder := filepath.Join(dir, "Talk [dQw4w9WgXcQ]")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{file, filepath.Join(folder, "01 Intro.m4a"), filepath.Join(folder, "02 Docs.m4a")} {
		if err := os.WriteFile(name, []byte("audio"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	job := finishedJob(t, store, "https://youtu.be/dQw4w9WgXcQ", domain.Completed, file)
	job.SplitChapters = true
	if err := store.Update(context.Background(), job, domain.Completed); err != nil {
		t.Fatal(err)
	}

	// A file of the user's own in the folder is kept, and so is the folder.
	notes := filepath.Join(folder, "my notes.txt")
	if err := os.WriteFile(notes, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/jobs/"+job.ID+"?delete_file=true", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", response.Code, response.Body.String())
	}
	for _, gone := range []string{file, filepath.Join(folder, "01 Intro.m4a"), filepath.Join(folder, "02 Docs.m4a")} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Errorf("%s was kept", gone)
		}
	}
	if _, err := os.Stat(notes); err != nil {
		t.Error("a file of the user's own was deleted")
	}

	// Without other files, the folder goes too.
	if err := os.Remove(notes); err != nil {
		t.Fatal(err)
	}
	if _, err := removeOutput(job, file); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Error("the empty chapter folder was kept")
	}
}

func TestLibraryFiles(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	file := filepath.Join(dir, "Title [dQw4w9WgXcQ] 1080p.mp4")
	if err := os.WriteFile(file, make([]byte, 1234), 0o600); err != nil {
		t.Fatal(err)
	}
	here := finishedJob(t, store, "https://youtu.be/dQw4w9WgXcQ", domain.Completed, file)
	gone := finishedJob(t, store, "https://youtu.be/jNQXAC9IVRw", domain.Completed, filepath.Join(dir, "Gone [jNQXAC9IVRw] 720p.mp4"))
	finishedJob(t, store, "https://youtu.be/aqz-KE-bpKQ", domain.Failed, "")

	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/files", nil))
	var body struct {
		Files map[string]libraryFile `json:"files"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &body) != nil {
		t.Fatalf("files = %d %s", response.Code, response.Body.String())
	}
	if len(body.Files) != 2 || body.Files[here.ID] != (libraryFile{Bytes: 1234}) || !body.Files[gone.ID].Missing {
		t.Fatalf("files = %+v", body.Files)
	}
}

func TestDeletingAFileTakesItsSidecars(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	file := write("Talk [jNQXAC9IVRw] 720p.mp4")
	gone := []string{"Talk [jNQXAC9IVRw] 720p.info.json", "Talk [jNQXAC9IVRw] 720p.description", "Talk [jNQXAC9IVRw] 720p.jpg", "Talk [jNQXAC9IVRw] 720p.en.srt", "Talk [jNQXAC9IVRw] 720p.webp"}
	kept := []string{"Talk [jNQXAC9IVRw] 1080p.mp4", "Talk [jNQXAC9IVRw] 1080p.jpg", "Talk [jNQXAC9IVRw] 720p.mkv", "Talk [jNQXAC9IVRw] 720p notes.txt", "Other.jpg"}
	for _, name := range append(append([]string{}, gone...), kept...) {
		write(name)
	}
	job := domain.Job{VideoID: "jNQXAC9IVRw", State: domain.Completed}
	if removed, err := removeOutput(job, file); err != nil || !removed {
		t.Fatalf("removeOutput = %v, %v", removed, err)
	}
	for _, name := range gone {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was kept", name)
		}
	}
	for _, name := range kept {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed", name)
		}
	}
}

func TestLibraryArchiveExport(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	finishedJob(t, store, "https://youtu.be/jNQXAC9IVRw", domain.Completed, filepath.Join(dir, "A [jNQXAC9IVRw].m4a"))
	finishedJob(t, store, "https://www.youtube.com/watch?v=jNQXAC9IVRw", domain.Completed, filepath.Join(dir, "A [jNQXAC9IVRw] 720p.mp4"))
	finishedJob(t, store, "https://vimeo.com/22439234", domain.Completed, filepath.Join(dir, "B [22439234].mp4"))
	finishedJob(t, store, "https://youtu.be/aqz-KE-bpKQ", domain.Failed, "")
	finishedJob(t, store, "https://x.com/a/status/2105708732323909827", domain.Completed, filepath.Join(dir, "C [2105708732323909827].mp4"))

	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/archive", nil))
	lines := strings.Split(strings.TrimSpace(response.Body.String()), "\n")
	slices.Sort(lines)
	if response.Code != http.StatusOK || !slices.Equal(lines, []string{"vimeo 22439234", "youtube jNQXAC9IVRw"}) {
		t.Fatalf("archive = %d %q", response.Code, response.Body.String())
	}
}
