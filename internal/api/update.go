package api

import (
	"context"
	"errors"
	"net/http"
)

var (
	ErrDownloadsRunning = errors.New("Finish or cancel the running downloads, then update yt-dlp.")
	ErrUpdateBusy       = errors.New("An update is already running.")
)

// YtdlpUpdate reports the result of updating yt-dlp.
type YtdlpUpdate struct {
	Version  string `json:"version"`
	Previous string `json:"previous,omitempty"`
	Updated  bool   `json:"updated"` // false when the installed version was already the latest
}

// YtdlpUpdater is optionally implemented by the settings value.
type YtdlpUpdater interface {
	UpdateYtdlp(context.Context) (YtdlpUpdate, error)
}

func addUpdateRoute(mux *http.ServeMux, updater YtdlpUpdater) {
	mux.HandleFunc("POST /api/system/update-ytdlp", func(w http.ResponseWriter, r *http.Request) {
		result, err := updater.UpdateYtdlp(r.Context())
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, result)
		case errors.Is(err, ErrDownloadsRunning):
			writeError(w, http.StatusConflict, "downloads_running", err.Error())
		case errors.Is(err, ErrUpdateBusy):
			writeError(w, http.StatusConflict, "update_busy", err.Error())
		default:
			if r.Context().Err() == nil {
				writeError(w, http.StatusBadGateway, "update_failed", "yt-dlp could not be updated: "+err.Error())
			}
		}
	})
}
