package api

import (
	"context"
	"encoding/json"
	"net/http"

	"ytgrab/internal/app/deps"
	"ytgrab/web"
)

// NewHandler wires the foundation API. More routes can be added as milestones
// are implemented without changing the health response contract.
func NewHandler(check func(context.Context) deps.Report, jobs JobStore, controller JobController, settings ...Settings) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/system/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(check(r.Context()))
	})
	mux.Handle("/", http.FileServer(http.FS(web.Static())))
	if jobs != nil {
		addJobRoutes(mux, jobs, controller)
	}
	if len(settings) != 0 && settings[0] != nil {
		addSettingsRoutes(mux, settings[0])
	}
	return protectLocalAPI(mux)
}
