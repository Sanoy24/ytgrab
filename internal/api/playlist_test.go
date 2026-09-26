package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ytgrab/internal/app/deps"
	"ytgrab/internal/domain"
	"ytgrab/internal/downloader/ytdlp"
	sqlitestore "ytgrab/internal/store/sqlite"
)

type fakePlaylistInspector struct{ fakeInspector }

func (fakePlaylistInspector) ListPlaylist(_ context.Context, url string) (ytdlp.Playlist, error) {
	if _, _, err := domain.ParsePlaylistURL(url); err != nil {
		return ytdlp.Playlist{}, err
	}
	return ytdlp.Playlist{ID: "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", Title: "Lectures", Entries: []ytdlp.PlaylistEntry{{VideoID: "dQw4w9WgXcQ", Title: "One"}}}, nil
}

func (fakePlaylistInspector) CachedTitle(videoID string) string {
	if videoID == "dQw4w9WgXcQ" {
		return "One"
	}
	return ""
}

func TestPlaylistListingAndConfirmedJobs(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandlerWithInspector(func(context.Context) deps.Report { return deps.Report{} }, store, nil, fakePlaylistInspector{})
	request := func(method, target, body string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, bytes.NewBufferString(body)))
		return response
	}

	if got := request(http.MethodGet, "/api/playlist?url=https://www.youtube.com/playlist?list=PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", ""); got.Code != http.StatusOK || !bytes.Contains(got.Body.Bytes(), []byte(`"video_id":"dQw4w9WgXcQ"`)) {
		t.Fatalf("list = %d: %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodGet, "/api/playlist?url=https://www.youtube.com/watch?v=dQw4w9WgXcQ%26list=RDdQw4w9WgXcQ", ""); got.Code != http.StatusBadRequest || !bytes.Contains(got.Body.Bytes(), []byte(`"code":"mix_playlist"`)) {
		t.Fatalf("mix = %d: %s", got.Code, got.Body.String())
	}

	// One bad ID rejects the whole request before any job is created.
	if got := request(http.MethodPost, "/api/playlist/jobs", `{"video_ids":["dQw4w9WgXcQ","bad&id"],"preset":"audio-m4a"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("bad ID = %d: %s", got.Code, got.Body.String())
	}
	var tooMany []string
	for i := range domain.MaxPlaylistItems + 1 {
		tooMany = append(tooMany, fmt.Sprintf(`"video%06d"`, i))
	}
	if got := request(http.MethodPost, "/api/playlist/jobs", `{"video_ids":[`+strings.Join(tooMany, ",")+`],"preset":"audio-m4a"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("too many = %d: %s", got.Code, got.Body.String())
	}
	if got := request(http.MethodPost, "/api/playlist/jobs", `{"video_ids":["dQw4w9WgXcQ"]}`); got.Code != http.StatusBadRequest {
		t.Fatalf("missing preset = %d: %s", got.Code, got.Body.String())
	}
	if jobs, _ := store.List(context.Background(), 10); len(jobs) != 0 {
		t.Fatalf("rejected requests created %d jobs", len(jobs))
	}

	created := request(http.MethodPost, "/api/playlist/jobs", `{"video_ids":["dQw4w9WgXcQ","jNQXAC9IVRw","dQw4w9WgXcQ"],"preset":"audio-m4a"}`)
	var result struct {
		Jobs    []domain.Job `json:"jobs"`
		Skipped int          `json:"skipped"`
	}
	if created.Code != http.StatusCreated || json.Unmarshal(created.Body.Bytes(), &result) != nil || len(result.Jobs) != 2 || result.Skipped != 1 {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	if result.Jobs[0].URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("job URL = %q", result.Jobs[0].URL)
	}
	if result.Jobs[0].Title == nil || *result.Jobs[0].Title != "One" || result.Jobs[1].Title != nil {
		t.Fatalf("listed titles were not applied: %v, %v", result.Jobs[0].Title, result.Jobs[1].Title)
	}
	// Videos already queued are skipped rather than failing the batch.
	again := request(http.MethodPost, "/api/playlist/jobs", `{"video_ids":["dQw4w9WgXcQ","aqz-KE-bpKQ"],"preset":"audio-m4a"}`)
	if again.Code != http.StatusCreated || json.Unmarshal(again.Body.Bytes(), &result) != nil || len(result.Jobs) != 1 || result.Skipped != 1 {
		t.Fatalf("repeat = %d: %s", again.Code, again.Body.String())
	}
}
