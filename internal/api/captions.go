package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/Sanoy24/ytgrab/internal/captions"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// Captioner makes subtitles on this computer for downloads that have none.
type Captioner interface {
	Status() captions.Status
	Install() error
	Request(ctx context.Context, jobID string, translate bool) error
}

// captionsProvider is optionally implemented by the settings value; Captions returns nil
// when local captions aren't available.
type captionsProvider interface {
	Captions() Captioner
}

func addCaptionRoutes(mux *http.ServeMux, captioner Captioner) {
	mux.HandleFunc("GET /api/captions", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, captioner.Status())
	})
	mux.HandleFunc("POST /api/captions/install", func(w http.ResponseWriter, _ *http.Request) {
		if err := captioner.Install(); err != nil {
			writeError(w, http.StatusConflict, "not_supported", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, captioner.Status())
	})
	mux.HandleFunc("POST /api/jobs/{id}/captions", func(w http.ResponseWriter, r *http.Request) {
		// ?translate=1 makes English captions of speech in another language.
		err := captioner.Request(r.Context(), r.PathValue("id"), r.URL.Query().Get("translate") == "1")
		switch {
		case err == nil:
			writeJSON(w, http.StatusAccepted, captioner.Status())
		case errors.Is(err, sqlitestore.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "That download no longer exists.")
		case errors.Is(err, captions.ErrNotInstalled), errors.Is(err, captions.ErrNoFile), errors.Is(err, captions.ErrBusy):
			writeError(w, http.StatusConflict, "captions_unavailable", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal", "Could not queue the captions.")
		}
	})
}
