package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

type JobStore interface {
	Create(context.Context, domain.Job) error
	Get(context.Context, string) (domain.Job, error)
	List(context.Context, int) ([]domain.Job, error)
	Update(context.Context, domain.Job, domain.State) error
}

type JobController interface {
	Wake()
	Cancel(string)
}

// Pauser is implemented by a queue that pauses while YouTube limits this network.
type Pauser interface {
	PausedUntil() (time.Time, bool)
	Resume()
}

func addJobRoutes(mux *http.ServeMux, store JobStore, controller JobController, inspector Inspector) {
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := store.List(r.Context(), 200)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not load downloads.")
			return
		}
		body := map[string]any{"jobs": jobs}
		if pauser, ok := controller.(Pauser); ok {
			if until, paused := pauser.PausedUntil(); paused {
				body["paused_until"] = until.UTC()
			}
		}
		writeJSON(w, http.StatusOK, body)
	})
	if pauser, ok := controller.(Pauser); ok {
		mux.HandleFunc("POST /api/system/resume", func(w http.ResponseWriter, _ *http.Request) {
			pauser.Resume()
			w.WriteHeader(http.StatusNoContent)
		})
	}
	mux.HandleFunc("POST /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			URL    string         `json:"url"`
			Preset *domain.Preset `json:"preset"`
			Format *struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			} `json:"format"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Send a URL and preset as JSON.")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_request", "Send exactly one JSON object.")
			return
		}
		if (input.Preset == nil) == (input.Format == nil) {
			writeError(w, http.StatusBadRequest, "invalid_request", "Choose one preset or inspected format.")
			return
		}
		var job domain.Job
		var err error
		if input.Format != nil {
			_, videoID, parseErr := domain.ParseVideoURL(input.URL)
			if parseErr != nil {
				writeError(w, http.StatusBadRequest, "invalid_url", parseErr.Error())
				return
			}
			selection := domain.FormatSelection{Kind: input.Format.Kind, ID: input.Format.ID}
			if !selection.Valid() {
				writeError(w, http.StatusBadRequest, "invalid_format", domain.ErrInvalidFormat.Error())
				return
			}
			if inspector == nil {
				writeError(w, http.StatusServiceUnavailable, "dependency_missing", "Format inspection is unavailable.")
				return
			}
			var found bool
			selection, found = inspector.Select(videoID, selection.Kind, selection.ID)
			if !found {
				writeError(w, http.StatusConflict, "inspection_required", "Check available formats again before adding this download.")
				return
			}
			job, err = domain.NewFormatJob(input.URL, selection)
		} else {
			job, err = domain.NewJob(input.URL, *input.Preset)
		}
		if errors.Is(err, domain.ErrInvalidURL) {
			writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}
		if errors.Is(err, domain.ErrInvalidPreset) {
			writeError(w, http.StatusBadRequest, "invalid_preset", err.Error())
			return
		}
		if errors.Is(err, domain.ErrInvalidFormat) {
			writeError(w, http.StatusBadRequest, "invalid_format", err.Error())
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "Could not create the download.")
			return
		}
		applyCachedTitle(inspector, &job)
		if err := store.Create(r.Context(), job); err != nil {
			if errors.Is(err, sqlitestore.ErrDuplicate) {
				writeError(w, http.StatusConflict, "duplicate_job", "This video is already in the queue.")
				return
			}
			writeError(w, http.StatusInternalServerError, "storage", "Could not save the download.")
			return
		}
		if controller != nil {
			controller.Wake()
		}
		writeJSON(w, http.StatusCreated, job)
	})
	mux.HandleFunc("GET /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("GET /api/jobs/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "stream_unavailable", "This server cannot stream progress.")
			return
		}
		streamOpened()
		defer streamClosed()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		writeEvent := func(job domain.Job) bool {
			payload, err := json.Marshal(job)
			if err != nil {
				return false
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}
		if !writeEvent(job) || !job.State.Active() {
			return
		}
		lastUpdate := job.UpdatedAt
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				job, err := store.Get(r.Context(), r.PathValue("id"))
				if err != nil {
					return
				}
				if !job.UpdatedAt.After(lastUpdate) {
					continue
				}
				if !writeEvent(job) || !job.State.Active() {
					return
				}
				lastUpdate = job.UpdatedAt
			}
		}
	})
	mux.HandleFunc("POST /api/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if !job.State.Active() {
			writeError(w, http.StatusConflict, "invalid_state", "Only queued or running jobs can be cancelled.")
			return
		}
		previous := job.State
		if err := job.Transition(domain.Cancelled); err != nil {
			writeError(w, http.StatusConflict, "invalid_state", err.Error())
			return
		}
		if err := store.Update(r.Context(), job, previous); err != nil {
			writeStoreError(w, err)
			return
		}
		if controller != nil {
			controller.Cancel(job.ID)
		}
		writeJSON(w, http.StatusOK, job)
	})
	mux.HandleFunc("POST /api/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if job.State != domain.Failed && job.State != domain.Cancelled {
			writeError(w, http.StatusConflict, "invalid_state", "Only failed or cancelled jobs can be retried.")
			return
		}
		previous := job.State
		if err := job.Transition(domain.Queued); err != nil {
			writeError(w, http.StatusConflict, "invalid_state", err.Error())
			return
		}
		if err := store.Update(r.Context(), job, previous); err != nil {
			writeStoreError(w, err)
			return
		}
		if controller != nil {
			controller.Wake()
		}
		writeJSON(w, http.StatusOK, job)
	})
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sqlitestore.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "That job no longer exists.")
	case errors.Is(err, sqlitestore.ErrConflict):
		writeError(w, http.StatusConflict, "invalid_state", "The job changed; refresh and try again.")
	case errors.Is(err, sqlitestore.ErrDuplicate):
		writeError(w, http.StatusConflict, "duplicate_job", "This video is already in the queue.")
	default:
		writeError(w, http.StatusInternalServerError, "storage", "Could not update the download.")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func protectLocalAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
				writeError(w, http.StatusForbidden, "cross_origin", "Open the local app to make changes.")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" {
				parsed, err := url.Parse(origin)
				if err != nil || parsed.Scheme != "http" || !strings.EqualFold(parsed.Host, r.Host) {
					writeError(w, http.StatusForbidden, "cross_origin", "Open the local app to make changes.")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
