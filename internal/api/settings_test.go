package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"ytgrab/internal/app/deps"
	"ytgrab/internal/settings"
	sqlitestore "ytgrab/internal/store/sqlite"
)

func TestSettingsAPI(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	defaultDir := t.TempDir()
	manager, err := settings.New(ctx, store, defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil, manager)
	request := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "/api/settings", bytes.NewBufferString(body)))
		return response
	}
	assertDirectory := func(got *httptest.ResponseRecorder, want string) {
		t.Helper()
		var body struct {
			DownloadsDir string `json:"downloads_dir"`
		}
		if err := json.Unmarshal(got.Body.Bytes(), &body); err != nil || got.Code != http.StatusOK || body.DownloadsDir != want {
			t.Fatalf("settings = %d: %s, %v", got.Code, got.Body.String(), err)
		}
	}
	assertDirectory(request(http.MethodGet, ""), defaultDir)
	if got := request(http.MethodPut, `{"downloads_dir":"relative"}`); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid folder = %d: %s", got.Code, got.Body.String())
	}
	selected := t.TempDir()
	body, err := json.Marshal(map[string]string{"downloads_dir": selected})
	if err != nil {
		t.Fatal(err)
	}
	if got := request(http.MethodPut, string(body)); got.Code != http.StatusOK {
		t.Fatalf("save folder = %d: %s", got.Code, got.Body.String())
	}
	assertDirectory(request(http.MethodGet, ""), selected)
	reloaded, err := settings.New(ctx, store, defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.DownloadsDir() != selected {
		t.Fatalf("reloaded settings = %q", reloaded.DownloadsDir())
	}
}
