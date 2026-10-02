package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

type countingController struct{ wakes, cancels int }

func (c *countingController) Wake()         { c.wakes++ }
func (c *countingController) Cancel(string) { c.cancels++ }

func TestPauseResumeAndMoveToTop(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	job.State = domain.Downloading
	if err := store.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	controller := &countingController{}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, controller)
	post := func(action string) (int, domain.Job) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/jobs/"+job.ID+"/"+action, bytes.NewReader(nil)))
		var got domain.Job
		_ = json.Unmarshal(response.Body.Bytes(), &got)
		return response.Code, got
	}
	if code, got := post("pause"); code != http.StatusOK || got.State != domain.Paused || controller.cancels != 1 {
		t.Fatalf("pause = %d %s, cancels %d", code, got.State, controller.cancels)
	}
	if code, _ := post("pause"); code != http.StatusConflict {
		t.Fatalf("pausing twice = %d", code)
	}
	if code, got := post("top"); code != http.StatusOK || got.Priority == 0 {
		t.Fatalf("top = %d priority %d", code, got.Priority)
	}
	if code, got := post("resume"); code != http.StatusOK || got.State != domain.Queued || controller.wakes == 0 {
		t.Fatalf("resume = %d %s, wakes %d", code, got.State, controller.wakes)
	}
	if code, _ := post("resume"); code != http.StatusConflict {
		t.Fatalf("resuming a queued job = %d", code)
	}
}
