package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

type PlaylistLister interface {
	ListPlaylist(ctx context.Context, url string, start int) (ytdlp.Playlist, error)
}

// titleSource is optionally implemented by inspectors that remember titles from recent
// inspections and playlist listings, so new jobs show a title while queued.
type titleSource interface {
	CachedTitle(videoID string) string
}

func applyCachedTitle(source any, job *domain.Job) {
	titles, ok := source.(titleSource)
	if !ok || job.Title != nil {
		return
	}
	if title := titles.CachedTitle(job.VideoID); title != "" {
		job.Title = &title
	}
}

// addPlaylistRoutes lists a playlist for review, then creates one preset job per
// confirmed video. The server builds every job URL from a validated video ID.
func addPlaylistRoutes(mux *http.ServeMux, lister PlaylistLister, store JobStore, controller JobController) {
	mux.HandleFunc("GET /api/playlist", func(w http.ResponseWriter, r *http.Request) {
		start := 1
		if value := r.URL.Query().Get("start"); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 100_000 {
				writeError(w, http.StatusBadRequest, "invalid_request", "The start position must be a positive number.")
				return
			}
			start = n
		}
		list, err := lister.ListPlaylist(r.Context(), r.URL.Query().Get("url"), start)
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, list)
		case errors.Is(err, domain.ErrMixPlaylist):
			writeError(w, http.StatusBadRequest, "mix_playlist", err.Error())
		case errors.Is(err, domain.ErrInvalidPlaylistURL):
			writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
		default:
			writeToolError(w, r, err, "Could not read the playlist. Check your connection and retry.")
		}
	})
	mux.HandleFunc("POST /api/playlist/jobs", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			VideoIDs []string       `json:"video_ids"`
			Preset   *domain.Preset `json:"preset"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_request", "Send video IDs and a preset as one JSON object.")
			return
		}
		if input.Preset == nil {
			writeError(w, http.StatusBadRequest, "invalid_preset", "Choose a preset for the playlist videos.")
			return
		}
		if len(input.VideoIDs) == 0 || len(input.VideoIDs) > domain.MaxPlaylistJobs {
			writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("Choose between 1 and %d videos.", domain.MaxPlaylistJobs))
			return
		}
		// Validate everything first so a bad request creates no jobs.
		var jobs []domain.Job
		seen := map[string]bool{}
		skipped := 0
		for _, id := range input.VideoIDs {
			if !domain.ValidVideoID(id) {
				writeError(w, http.StatusBadRequest, "invalid_request", "One of the video IDs is not valid.")
				return
			}
			if seen[id] {
				skipped++
				continue
			}
			seen[id] = true
			job, err := domain.NewJob(domain.VideoURL(id), *input.Preset)
			if errors.Is(err, domain.ErrInvalidPreset) {
				writeError(w, http.StatusBadRequest, "invalid_preset", err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "Could not create the downloads.")
				return
			}
			applyCachedTitle(lister, &job)
			jobs = append(jobs, job)
		}
		created := []domain.Job{}
		for _, job := range jobs {
			if err := store.Create(r.Context(), job); err != nil {
				if errors.Is(err, sqlitestore.ErrDuplicate) {
					skipped++
					continue
				}
				writeError(w, http.StatusInternalServerError, "storage", "Could not save the downloads.")
				return
			}
			created = append(created, job)
		}
		if controller != nil && len(created) > 0 {
			controller.Wake()
		}
		writeJSON(w, http.StatusCreated, map[string]any{"jobs": created, "skipped": skipped})
	})
}

// writeToolError maps yt-dlp failures to stable HTTP statuses and error codes.
func writeToolError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	var toolError *ytdlp.Error
	if errors.As(err, &toolError) {
		status := http.StatusBadGateway
		switch toolError.Code {
		case "video_unavailable":
			status = http.StatusUnprocessableEntity
		case "blocked":
			status = http.StatusTooManyRequests
		case "dependency_missing":
			status = http.StatusServiceUnavailable
		}
		writeError(w, status, toolError.Code, toolError.Message)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	writeError(w, http.StatusBadGateway, "network", fallback)
}
