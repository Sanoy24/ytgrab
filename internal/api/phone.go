package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Sanoy24/ytgrab/internal/phone"
)

// PhoneAccess lets phones on the same Wi-Fi use YTGrab. These routes exist only on the
// computer: the phone server refuses them.
type PhoneAccess interface {
	Status() phone.Status
	SetEnabled(context.Context, bool) error
	NewPairing() (phone.Pairing, error)
	Forget(context.Context, string) error
}

// phoneProvider is optionally implemented by the settings value; Phones returns nil when
// phone access isn't available.
type phoneProvider interface {
	Phones() PhoneAccess
}

func addPhoneRoutes(mux *http.ServeMux, phones PhoneAccess) {
	mux.HandleFunc("GET /api/phone", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, phones.Status())
	})
	mux.HandleFunc("PUT /api/phone", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input); err != nil || input.Enabled == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Send {\"enabled\": true} or false.")
			return
		}
		if err := phones.SetEnabled(r.Context(), *input.Enabled); err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not save the setting.")
			return
		}
		writeJSON(w, http.StatusOK, phones.Status())
	})
	mux.HandleFunc("POST /api/phone/pair", func(w http.ResponseWriter, _ *http.Request) {
		pairing, err := phones.NewPairing()
		switch {
		case errors.Is(err, phone.ErrOff):
			writeError(w, http.StatusConflict, "phone_off", "Turn on phone access first.")
		case errors.Is(err, phone.ErrTooMany):
			writeError(w, http.StatusConflict, "too_many_phones", "Remove a phone before pairing another.")
		case errors.Is(err, phone.ErrNoAddress):
			writeError(w, http.StatusConflict, "no_network", "This computer isn't connected to a Wi-Fi or local network.")
		case err != nil:
			writeError(w, http.StatusInternalServerError, "pairing_failed", "Could not make a pairing code.")
		default:
			writeJSON(w, http.StatusOK, pairing)
		}
	})
	mux.HandleFunc("DELETE /api/phone/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := phones.Forget(r.Context(), r.PathValue("id")); err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not remove the phone.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
