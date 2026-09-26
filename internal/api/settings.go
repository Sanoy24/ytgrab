package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	settingspkg "ytgrab/internal/settings"
)

type Settings interface {
	DownloadsDir() string
	SetDownloadsDir(context.Context, string) error
}

func addSettingsRoutes(mux *http.ServeMux, settings Settings) {
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"downloads_dir": settings.DownloadsDir()})
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			DownloadsDir string `json:"downloads_dir"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Send an output folder as JSON.")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeError(w, http.StatusBadRequest, "invalid_request", "Send exactly one JSON object.")
			return
		}
		if err := settings.SetDownloadsDir(r.Context(), input.DownloadsDir); err != nil {
			if errors.Is(err, settingspkg.ErrInvalidDirectory) {
				writeError(w, http.StatusBadRequest, "invalid_directory", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "storage", "Could not save the output folder.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"downloads_dir": settings.DownloadsDir()})
	})
}
