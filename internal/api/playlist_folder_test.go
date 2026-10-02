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
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func TestPlaylistJobsGoIntoASafeSubfolder(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandlerWithInspector(func(context.Context) deps.Report { return deps.Report{} }, store, nil, fakePlaylistInspector{})
	post := func(body string) []map[string]any {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/playlist/jobs", bytes.NewBufferString(body)))
		if response.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", response.Code, response.Body.String())
		}
		var decoded struct {
			Jobs []map[string]any `json:"jobs"`
		}
		_ = json.Unmarshal(response.Body.Bytes(), &decoded)
		return decoded.Jobs
	}
	jobs := post(`{"video_ids":["aBcDeFgHiJk"],"preset":"audio-m4a","folder":"../../My: Playlist?"}`)
	if len(jobs) != 1 || jobs[0]["folder"] != "My Playlist" {
		t.Fatalf("jobs = %v", jobs)
	}
	jobs = post(`{"video_ids":["dQw4w9WgXcQ"],"preset":"audio-m4a"}`)
	if len(jobs) != 1 || jobs[0]["folder"] != nil {
		t.Fatalf("without a folder = %v", jobs)
	}
}
