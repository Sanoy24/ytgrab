package api

import (
	"context"
	"errors"
	"net/http"

	"ytgrab/internal/domain"
	"ytgrab/internal/downloader/ytdlp"
)

type Inspector interface {
	Inspect(context.Context, string) (ytdlp.Inspection, error)
	Select(videoID string, kind string, id string) (domain.FormatSelection, bool)
}

func addInspectRoute(mux *http.ServeMux, inspector Inspector) {
	mux.HandleFunc("GET /api/inspect", func(w http.ResponseWriter, r *http.Request) {
		result, err := inspector.Inspect(r.Context(), r.URL.Query().Get("url"))
		if err == nil {
			writeJSON(w, http.StatusOK, result)
			return
		}
		if errors.Is(err, domain.ErrInvalidURL) {
			writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
			return
		}
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
		writeError(w, http.StatusBadGateway, "network", "Could not inspect formats. Check your connection and retry.")
	})
}
