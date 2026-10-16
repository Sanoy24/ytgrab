package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func TestImportingAYtdlpArchive(t *testing.T) {
	store, err := sqlitestore.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := NewHandlerWithInspector(func(context.Context) deps.Report { return deps.Report{} }, store, nil, fakePlaylistInspector{})
	request := func(method, target, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, bytes.NewBufferString(body)))
		return response
	}
	playlist := "/api/playlist?url=https://www.youtube.com/playlist?list=PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb"
	if got := request(http.MethodGet, playlist, ""); strings.Contains(got.Body.String(), `"have":true`) {
		t.Fatalf("marked as had before importing: %s", got.Body.String())
	}

	archive := "youtube dQw4w9WgXcQ\nyoutube dQw4w9WgXcQ\nvimeo 76979871\ntwitter 2105708732323909827\nyoutube not-an-id!\n\n"
	got := request(http.MethodPost, "/api/library/archive", archive)
	var result map[string]int
	if err := json.Unmarshal(got.Body.Bytes(), &result); err != nil || got.Code != http.StatusOK {
		t.Fatalf("import = %d %s", got.Code, got.Body.String())
	}
	if result["added"] != 2 || result["skipped"] != 2 {
		t.Errorf("result = %v, want 2 added (one repeated line) and 2 skipped (X, a bad ID)", result)
	}
	if again := request(http.MethodPost, "/api/library/archive", archive); !strings.Contains(again.Body.String(), `"already":2`) {
		t.Errorf("importing again = %s", again.Body.String())
	}
	if empty := request(http.MethodPost, "/api/library/archive", "hello\n"); empty.Code != http.StatusBadRequest {
		t.Errorf("a file without videos = %d", empty.Code)
	}

	// The playlist review now marks the video as already downloaded.
	if got := request(http.MethodGet, playlist, ""); !strings.Contains(got.Body.String(), `"have":true`) {
		t.Fatalf("playlist after importing: %s", got.Body.String())
	}
}

func TestLibrarySearch(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	video := filepath.Join(dir, "Lecture 4 [jNQXAC9IVRw] 720p.mp4")
	_ = os.WriteFile(video, []byte("video"), 0o600)
	_ = os.WriteFile(strings.TrimSuffix(video, ".mp4")+".en.srt", []byte("1\n00:12:31,000 --> 00:12:33,000\nnow a binary search tree\n"), 0o600)
	job := finishedJob(t, store, "https://youtu.be/jNQXAC9IVRw", domain.Completed, video)
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil)
	search := func(q string) []searchResult {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/library/search?q="+url.QueryEscape(q), nil))
		var body struct {
			Results []searchResult `json:"results"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || response.Code != http.StatusOK {
			t.Fatalf("search %q = %d %s", q, response.Code, response.Body.String())
		}
		return body.Results
	}
	// Found right after the download finished: the search indexes what's new first.
	results := search("binary tre")
	if len(results) != 1 || results[0].JobID != job.ID || len(results[0].Matches) != 1 || *results[0].Matches[0].Start != 751 {
		t.Fatalf("results = %+v", results)
	}
	if got := search(`" OR *`); len(got) != 0 {
		t.Errorf("punctuation only = %+v", got)
	}
	if got := search("giraffe"); len(got) != 0 {
		t.Errorf("no match = %+v", got)
	}
}
