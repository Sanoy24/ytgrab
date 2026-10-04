package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/web"
)

// NewHandler wires the foundation API. More routes can be added as milestones
// are implemented without changing the health response contract.
func NewHandler(check func(context.Context) deps.Report, jobs JobStore, controller JobController, settings ...Settings) http.Handler {
	return NewHandlerWithInspector(check, jobs, controller, nil, settings...)
}

func NewHandlerWithInspector(check func(context.Context) deps.Report, jobs JobStore, controller JobController, inspector Inspector, settings ...Settings) http.Handler {
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
	// The page is built into the program; "no-cache" makes the browser check for a newer
	// copy each time, so an updated YTGrab never runs with the previous version's page.
	static := http.FileServer(http.FS(web.Static()))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		static.ServeHTTP(w, r)
	}))
	if jobs != nil {
		addJobRoutes(mux, jobs, controller, inspector)
		addHistoryRoutes(mux, jobs)
	}
	if inspector != nil {
		addInspectRoute(mux, inspector)
		if searcher, ok := inspector.(Searcher); ok {
			addSearchRoute(mux, searcher)
		}
		if lister, ok := inspector.(PlaylistLister); ok && jobs != nil {
			addPlaylistRoutes(mux, lister, jobs, controller)
		}
	}
	if len(settings) != 0 && settings[0] != nil {
		addSettingsRoutes(mux, settings[0])
		if updater, ok := settings[0].(YtdlpUpdater); ok {
			addUpdateRoute(mux, updater)
		}
		if reporter, ok := settings[0].(VersionReporter); ok {
			addVersionRoute(mux, reporter)
		}
		if updater, ok := settings[0].(YTGrabUpdater); ok {
			addYTGrabUpdateRoute(mux, updater)
		}
		if prefs, ok := settings[0].(backupSettings); ok {
			if store, ok := jobs.(backupStore); ok {
				version := ""
				if reporter, ok := settings[0].(VersionReporter); ok {
					version = reporter.YTGrabVersion().Version
				}
				addBackupRoutes(mux, store, prefs, version)
			}
		}
		if provider, ok := settings[0].(watchProvider); ok {
			if watches := provider.Watches(); watches != nil {
				addWatchRoutes(mux, watches)
			}
		}
	}
	return secureHeaders(protectLocalAPI(mux))
}
