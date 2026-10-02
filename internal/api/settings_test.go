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
	"github.com/Sanoy24/ytgrab/internal/picker"
	"github.com/Sanoy24/ytgrab/internal/settings"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

func TestSettingsAPI(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	defaultDir := t.TempDir()
	manager, err := settings.New(ctx, store, defaultDir, false)
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
	reloaded, err := settings.New(ctx, store, defaultDir, false)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.DownloadsDir() != selected {
		t.Fatalf("reloaded settings = %q", reloaded.DownloadsDir())
	}
}

type fakePicker struct {
	path string
	err  error
}

func (fake fakePicker) Available() bool { return fake.err == nil || fake.err != picker.ErrUnavailable }
func (fake fakePicker) Pick(context.Context, string) (string, error) {
	return fake.path, fake.err
}

type settingsWithPicker struct {
	*settings.Manager
	fakePicker
}

func TestFirstRunFolderChoice(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	suggested := filepath.Join(t.TempDir(), "Downloads", "ytgrab")
	manager, err := settings.New(ctx, store, suggested, false)
	if err != nil {
		t.Fatal(err)
	}
	chosen := t.TempDir()
	current := &settingsWithPicker{Manager: manager, fakePicker: fakePicker{err: picker.ErrCancelled}}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil, current)
	request := func(method, target string) (int, map[string]any) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, nil))
		var body map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &body)
		return response.Code, body
	}

	code, body := request(http.MethodGet, "/api/settings")
	if code != http.StatusOK || body["configured"] != false || body["default_dir"] != suggested || body["can_pick"] != true {
		t.Fatalf("first run settings = %d %v", code, body)
	}
	code, body = request(http.MethodPost, "/api/settings/pick-folder")
	if code != http.StatusOK || body["cancelled"] != true || body["configured"] != false {
		t.Fatalf("cancelled pick = %d %v", code, body)
	}
	current.fakePicker = fakePicker{path: chosen}
	code, body = request(http.MethodPost, "/api/settings/pick-folder")
	if code != http.StatusOK || body["downloads_dir"] != chosen || body["configured"] != true {
		t.Fatalf("picked folder = %d %v", code, body)
	}
	current.fakePicker = fakePicker{err: picker.ErrUnavailable}
	if code, body = request(http.MethodPost, "/api/settings/pick-folder"); code != http.StatusNotImplemented || body["error"] == nil {
		t.Fatalf("unavailable picker = %d %v", code, body)
	}
	if code, body = request(http.MethodPost, "/api/settings/use-default"); code != http.StatusOK || body["downloads_dir"] != suggested {
		t.Fatalf("use default = %d %v", code, body)
	}
}

// A DNS-rebinding page is same-origin with its own host name; only loopback hosts pass.
func TestRejectsNonLoopbackHost(t *testing.T) {
	handler := RequireLoopbackHost(NewHandler(func(context.Context) deps.Report { return deps.Report{} }, nil, nil))
	for host, want := range map[string]int{"127.0.0.1:8787": http.StatusOK, "localhost:8787": http.StatusOK, "[::1]:8787": http.StatusOK, "evil.example:8787": http.StatusForbidden} {
		request := httptest.NewRequest(http.MethodGet, "/api/system/health", nil)
		request.Host = host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != want {
			t.Errorf("Host %s = %d, want %d", host, response.Code, want)
		}
	}
}

func TestBrowserSignInSetting(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := settings.New(ctx, store, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil, manager)
	request := func(method, target, body string) (int, map[string]any) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, bytes.NewBufferString(body)))
		var decoded map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &decoded)
		return response.Code, decoded
	}
	code, body := request(http.MethodGet, "/api/settings", "")
	if code != http.StatusOK || body["cookies_browser"] != "" || len(body["cookie_browsers"].([]any)) == 0 {
		t.Fatalf("settings = %d %v", code, body)
	}
	if code, _ := request(http.MethodPut, "/api/settings/cookies", `{"browser":"firefox --x"}`); code != http.StatusBadRequest {
		t.Fatalf("invalid browser = %d", code)
	}
	if code, body := request(http.MethodPut, "/api/settings/cookies", `{"browser":"firefox"}`); code != http.StatusOK || body["cookies_browser"] != "firefox" {
		t.Fatalf("set firefox = %d %v", code, body)
	}
	if code, body := request(http.MethodPut, "/api/settings/cookies", `{"browser":""}`); code != http.StatusOK || body["cookies_browser"] != "" {
		t.Fatalf("turn off = %d %v", code, body)
	}
}

func TestDownloadPreferencesAPI(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := settings.New(ctx, store, t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil, manager)
	request := func(method, target, body string) (int, map[string]any) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, target, bytes.NewBufferString(body)))
		var decoded map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &decoded)
		return response.Code, decoded
	}
	if code, body := request(http.MethodGet, "/api/settings", ""); code != http.StatusOK || body["max_downloads"] != float64(2) || body["default_preset"] != "video-best" {
		t.Fatalf("settings = %d %v", code, body)
	}
	if code, _ := request(http.MethodPut, "/api/settings/preferences", `{"max_downloads":9}`); code != http.StatusBadRequest {
		t.Fatalf("max_downloads 9 = %d", code)
	}
	if code, body := request(http.MethodPut, "/api/settings/preferences", `{"max_downloads":1}`); code != http.StatusOK || body["max_downloads"] != float64(1) || body["default_preset"] != "video-best" {
		t.Fatalf("set one field = %d %v", code, body)
	}
	if code, body := request(http.MethodPut, "/api/settings/preferences", `{"default_preset":"audio-m4a"}`); code != http.StatusOK || body["default_preset"] != "audio-m4a" || body["max_downloads"] != float64(1) {
		t.Fatalf("set preset = %d %v", code, body)
	}
}
