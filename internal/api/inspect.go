package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
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
		writeToolError(w, r, err, "Could not inspect formats. Check your connection and retry.")
	})
}
