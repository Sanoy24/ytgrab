package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Sanoy24/ytgrab/internal/notify"
)

func addNotifyRoutes(mux *http.ServeMux, settings notifySettings) {
	mux.HandleFunc("PUT /api/settings/notify", func(w http.ResponseWriter, r *http.Request) {
		var input notify.Settings
		if !decodeOne(w, r, &input) {
			return
		}
		if err := settings.SetNotify(r.Context(), input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_notify", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, settings.Notify(r.Context()))
	})
	// Sends a test notification with the given settings (saved or not), so the user sees
	// it arrive before relying on it.
	mux.HandleFunc("POST /api/settings/notify/test", func(w http.ResponseWriter, r *http.Request) {
		var input notify.Settings
		if !decodeOne(w, r, &input) {
			return
		}
		if input.Kind == "" {
			writeError(w, http.StatusBadRequest, "invalid_notify", "Choose a service first.")
			return
		}
		if err := input.Valid(); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_notify", err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		message := notify.Message{Event: "test", Title: "Test notification", Body: "YTGrab will tell you here when downloads finish or fail."}
		if err := notify.Send(ctx, input, message); err != nil {
			writeError(w, http.StatusBadGateway, "notify_failed", "The test didn't arrive: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
