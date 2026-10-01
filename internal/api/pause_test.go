package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

type pausingController struct {
	until   time.Time
	resumed bool
}

func (*pausingController) Wake()         {}
func (*pausingController) Cancel(string) {}
func (c *pausingController) PausedUntil() (time.Time, bool) {
	return c.until, !c.resumed && time.Now().Before(c.until)
}
func (c *pausingController) Resume() { c.resumed = true }

func TestJobListReportsPauseAndResumeEndsIt(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	controller := &pausingController{until: time.Now().Add(14 * time.Minute)}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, controller)
	list := func() map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/jobs", nil))
		var body map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	if body := list(); body["paused_until"] == nil {
		t.Fatalf("paused list = %v", body)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/system/resume", nil))
	if response.Code != http.StatusNoContent || !controller.resumed {
		t.Fatalf("resume = %d, resumed %v", response.Code, controller.resumed)
	}
	if body := list(); body["paused_until"] != nil {
		t.Fatalf("list after resume = %v", body)
	}
}
