package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
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
