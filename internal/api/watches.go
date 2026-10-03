package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
	"github.com/Sanoy24/ytgrab/internal/watch"
)

// Watcher follows channels and playlists and queues their new videos.
type Watcher interface {
	List(context.Context) ([]domain.Watch, error)
	Add(ctx context.Context, url string, preset domain.Preset, folder bool, backfill int) (domain.Watch, error)
	Check(ctx context.Context, id string) (domain.Watch, error)
	Update(ctx context.Context, id string, preset *domain.Preset, folder, paused *bool) (domain.Watch, error)
	Delete(ctx context.Context, id string) error
}

// watchProvider is optionally implemented by the settings value; Watches returns nil when
// watching isn't available.
type watchProvider interface {
	Watches() Watcher
}

func writeWatchError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidWatchURL), errors.Is(err, domain.ErrInvalidPlaylistURL):
		writeError(w, http.StatusBadRequest, "invalid_url", domain.ErrInvalidWatchURL.Error())
	case errors.Is(err, domain.ErrMixPlaylist):
		writeError(w, http.StatusBadRequest, "mix_playlist", err.Error())
	case errors.Is(err, domain.ErrInvalidPreset):
		writeError(w, http.StatusBadRequest, "invalid_preset", err.Error())
	case errors.Is(err, watch.ErrInvalidBackfill):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, sqlitestore.ErrWatchExists):
		writeError(w, http.StatusConflict, "watch_exists", err.Error())
	case errors.Is(err, sqlitestore.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "That channel or playlist isn't being watched.")
	default:
		writeToolError(w, r, err, "Could not read the channel or playlist. Check your connection and retry.")
	}
}

func decodeOne(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_request", "Send one JSON object with the listed fields.")
		return false
	}
	return true
}

func addWatchRoutes(mux *http.ServeMux, watches Watcher) {
	mux.HandleFunc("GET /api/watches", func(w http.ResponseWriter, r *http.Request) {
		list, err := watches.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not load the watched channels.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"watches":        list,
			"interval_hours": int(watch.Interval.Hours()),
			"max_backfill":   watch.MaxBackfill,
			"max_per_check":  watch.MaxPerCheck,
		})
	})
	mux.HandleFunc("POST /api/watches", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			URL      string        `json:"url"`
			Preset   domain.Preset `json:"preset"`
			Folder   bool          `json:"folder"`
			Backfill int           `json:"backfill"`
		}
		if !decodeOne(w, r, &input) {
			return
		}
		added, err := watches.Add(r.Context(), input.URL, input.Preset, input.Folder, input.Backfill)
		if err != nil {
			writeWatchError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, added)
	})
	mux.HandleFunc("POST /api/watches/{id}/check", func(w http.ResponseWriter, r *http.Request) {
		checked, err := watches.Check(r.Context(), r.PathValue("id"))
		if err != nil {
			writeWatchError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, checked)
	})
	mux.HandleFunc("PUT /api/watches/{id}", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Preset *domain.Preset `json:"preset"`
			Folder *bool          `json:"folder"`
			Paused *bool          `json:"paused"`
		}
		if !decodeOne(w, r, &input) {
			return
		}
		updated, err := watches.Update(r.Context(), r.PathValue("id"), input.Preset, input.Folder, input.Paused)
		if err != nil {
			writeWatchError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	})
	mux.HandleFunc("DELETE /api/watches/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := watches.Delete(r.Context(), r.PathValue("id")); err != nil {
			writeWatchError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
