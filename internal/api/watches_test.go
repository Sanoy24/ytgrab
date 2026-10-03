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
	"github.com/Sanoy24/ytgrab/internal/settings"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
	"github.com/Sanoy24/ytgrab/internal/watch"
)

// fakeWatcher keeps watches in memory and checks inputs like the real service.
type fakeWatcher struct{ watches map[string]domain.Watch }

func (f *fakeWatcher) List(context.Context) ([]domain.Watch, error) {
	out := []domain.Watch{}
	for _, w := range f.watches {
		out = append(out, w)
	}
	return out, nil
}
func (f *fakeWatcher) Add(_ context.Context, url string, opts watch.Options) (domain.Watch, error) {
	if opts.Backfill < 0 || opts.Backfill > watch.MaxBackfill {
		return domain.Watch{}, watch.ErrInvalidBackfill
	}
	if !domain.ValidWatchOptions(opts.MinMinutes, opts.Keywords, opts.IntervalHours) {
		return domain.Watch{}, domain.ErrInvalidWatchOptions
	}
	w, err := domain.NewWatch(url, opts.Preset)
	if err != nil {
		return w, err
	}
	for _, existing := range f.watches {
		if existing.URL == w.URL {
			return domain.Watch{}, sqlitestore.ErrWatchExists
		}
	}
	w.Folder, w.Title, w.MinMinutes = opts.Folder, "Google for Developers", opts.MinMinutes
	f.watches[w.ID] = w
	return w, nil
}
func (f *fakeWatcher) Check(_ context.Context, id string) (domain.Watch, error) {
	w, ok := f.watches[id]
	if !ok {
		return w, sqlitestore.ErrNotFound
	}
	w.LastNew = 1
	return w, nil
}
func (f *fakeWatcher) Update(_ context.Context, id string, changes watch.Changes) (domain.Watch, error) {
	paused := changes.Paused
	w, ok := f.watches[id]
	if !ok {
		return w, sqlitestore.ErrNotFound
	}
	if paused != nil {
		w.Paused = *paused
	}
	f.watches[id] = w
	return w, nil
}
func (f *fakeWatcher) Delete(_ context.Context, id string) error {
	if _, ok := f.watches[id]; !ok {
		return sqlitestore.ErrNotFound
	}
	delete(f.watches, id)
	return nil
}

type settingsWithWatches struct {
	*settings.Manager
	watcher Watcher
}

func (s settingsWithWatches) Watches() Watcher { return s.watcher }

func TestWatchRoutes(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.Open(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, _ := settings.New(ctx, store, t.TempDir(), true)
	handler := NewHandler(func(context.Context) deps.Report { return deps.Report{} }, store, nil,
		settingsWithWatches{Manager: manager, watcher: &fakeWatcher{watches: map[string]domain.Watch{}}})
	call := func(method, path, body string) (int, map[string]any) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		var decoded map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &decoded)
		return response.Code, decoded
	}
	code, added := call(http.MethodPost, "/api/watches", `{"url":"https://www.youtube.com/@GoogleDevelopers","preset":"audio-m4a","folder":true,"backfill":1,"min_minutes":2,"keywords":"gemma","interval_hours":24}`)
	if code != http.StatusCreated || added["kind"] != "channel" || added["min_minutes"] != float64(2) {
		t.Fatalf("add = %d %v", code, added)
	}
	id := added["id"].(string)
	for body, want := range map[string]int{
		`{"url":"https://www.youtube.com/@GoogleDevelopers/videos","preset":"audio-m4a"}`:       http.StatusConflict,
		`{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","preset":"audio-m4a"}`:            http.StatusBadRequest,
		`{"url":"https://www.youtube.com/@ChromeDevs","preset":"audio-m4a","backfill":99}`:      http.StatusBadRequest,
		`{"url":"https://www.youtube.com/@ChromeDevs","preset":"--exec"}`:                       http.StatusBadRequest,
		`{"url":"https://www.youtube.com/@ChromeDevs","preset":"audio-m4a","x":1}`:              http.StatusBadRequest,
		`{"url":"https://www.youtube.com/@ChromeDevs","preset":"audio-m4a","interval_hours":3}`: http.StatusBadRequest,
	} {
		if code, _ := call(http.MethodPost, "/api/watches", body); code != want {
			t.Errorf("%s = %d, want %d", body, code, want)
		}
	}
	if code, list := call(http.MethodGet, "/api/watches", ""); code != http.StatusOK || len(list["watches"].([]any)) != 1 || list["interval_hours"] != float64(6) {
		t.Fatalf("list = %d %v", code, list)
	}
	if code, got := call(http.MethodPost, "/api/watches/"+id+"/check", ""); code != http.StatusOK || got["last_new"] != float64(1) {
		t.Fatalf("check = %d %v", code, got)
	}
	if code, got := call(http.MethodPut, "/api/watches/"+id, `{"paused":true}`); code != http.StatusOK || got["paused"] != true {
		t.Fatalf("pause = %d %v", code, got)
	}
	if code, _ := call(http.MethodDelete, "/api/watches/"+id, ""); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	if code, _ := call(http.MethodPost, "/api/watches/"+id+"/check", ""); code != http.StatusNotFound {
		t.Fatalf("check after delete = %d", code)
	}
}
