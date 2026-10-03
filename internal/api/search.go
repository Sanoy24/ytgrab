package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/Sanoy24/ytgrab/internal/downloader/ytdlp"
)

// Searcher is optionally implemented by the inspector.
type Searcher interface {
	Search(ctx context.Context, query string) ([]ytdlp.SearchResult, error)
}

func addSearchRoute(mux *http.ServeMux, searcher Searcher) {
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		results, err := searcher.Search(r.Context(), r.URL.Query().Get("q"))
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, map[string]any{"results": results})
		case errors.Is(err, ytdlp.ErrInvalidQuery):
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error())
		default:
			writeToolError(w, r, err, "Could not search YouTube. Check your connection and retry.")
		}
	})
}
