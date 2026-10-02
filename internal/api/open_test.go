package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func TestOpenOnlyAFinishedDownloadsOwnFile(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var opened []string
	original := openFile
	openFile = func(path string) error { opened = append(opened, path); return nil }
	t.Cleanup(func() { openFile = original })

	dir := t.TempDir()
	saved := filepath.Join(dir, "Me at the zoo [jNQXAC9IVRw] 130k.m4a")
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{saved, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	add := func(state domain.State, path string) string {
		job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
		job.State, job.OutputPath = state, &path
		if err := store.Create(ctx, job); err != nil {
			t.Fatal(err)
		}
		return job.ID
	}
	done := add(domain.Completed, saved)
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	post := func(id string) int {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/jobs/"+id+"/open", nil))
		return response.Code
	}
	if code := post(done); code != http.StatusNoContent || len(opened) != 1 || opened[0] != saved {
		t.Fatalf("open = %d, opened %v", code, opened)
	}
	if err := store.Delete(ctx, done); err != nil {
		t.Fatal(err)
	}
	// A path that isn't this download's file is refused.
	if code := post(add(domain.Completed, other)); code != http.StatusConflict || len(opened) != 1 {
		t.Fatalf("other file = %d, opened %v", code, opened)
	}
	if code := post("job_missing"); code != http.StatusNotFound {
		t.Fatalf("missing job = %d", code)
	}
}
