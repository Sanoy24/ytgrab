package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/picker"
	settingspkg "github.com/Sanoy24/ytgrab/internal/settings"
)

type Settings interface {
	DownloadsDir() string
	SetDownloadsDir(context.Context, string) error
}

// firstRunSettings is implemented by settings that know whether a folder was chosen.
type firstRunSettings interface {
	Configured() bool
	DefaultDir() string
	UseDefault(context.Context) error
}

// cookieSettings is implemented by settings that support browser sign-in.
type cookieSettings interface {
	CookiesBrowser() string
	SetCookiesBrowser(context.Context, string) error
}

// preferenceSettings is implemented by settings with download preferences.
type preferenceSettings interface {
	MaxDownloads() int
	SetMaxDownloads(context.Context, int) error
	DefaultPreset() domain.Preset
	SetDefaultPreset(context.Context, domain.Preset) error
}

// speedSettings is implemented by settings with a per-download speed limit.
type speedSettings interface {
	SpeedLimit() int
	SetSpeedLimit(context.Context, int) error
}

// subtitleSettings is implemented by settings that can add subtitles to video downloads.
type subtitleSettings interface {
	Subtitles() (mode, lang string)
	SetSubtitles(ctx context.Context, mode, lang string) error
}

// startupSettings is implemented by settings that can start YTGrab when the user signs in.
type startupSettings interface {
	StartAtLoginSupported() bool
	StartAtLogin() bool
	SetStartAtLogin(bool) error
	StartAtLoginLabel() string
}

// FolderPicker is optionally implemented by the settings value to open the operating
// system's folder window on this computer.
type FolderPicker interface {
	Available() bool
	Pick(ctx context.Context, initial string) (string, error)
}

func settingsBody(settings Settings) map[string]any {
	body := map[string]any{"downloads_dir": settings.DownloadsDir(), "configured": true, "can_pick": false}
	if first, ok := settings.(firstRunSettings); ok {
		body["configured"] = first.Configured()
		body["default_dir"] = first.DefaultDir()
	}
	if folders, ok := settings.(FolderPicker); ok {
		body["can_pick"] = folders.Available()
	}
	if prefs, ok := settings.(preferenceSettings); ok {
		body["max_downloads"] = prefs.MaxDownloads()
		body["max_downloads_limit"] = settingspkg.MaxDownloadsCap
		body["default_preset"] = prefs.DefaultPreset()
	}
	if cookies, ok := settings.(cookieSettings); ok {
		body["cookies_browser"] = cookies.CookiesBrowser()
		body["cookie_browsers"] = settingspkg.CookieBrowsers
	}
	if startup, ok := settings.(startupSettings); ok && startup.StartAtLoginSupported() {
		body["start_at_login"] = startup.StartAtLogin()
		body["start_at_login_label"] = startup.StartAtLoginLabel()
	}
	if speed, ok := settings.(speedSettings); ok {
		body["speed_limit_kbps"] = speed.SpeedLimit()
		body["speed_limits"] = settingspkg.SpeedLimits
	}
	if subtitles, ok := settings.(subtitleSettings); ok {
		body["subtitles_mode"], body["subtitles_lang"] = subtitles.Subtitles()
		body["subtitle_languages"] = settingspkg.SubtitleLanguages
	}
	return body
}

func writeSettingsError(w http.ResponseWriter, err error) {
	if errors.Is(err, settingspkg.ErrInvalidDirectory) {
		writeError(w, http.StatusBadRequest, "invalid_directory", err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "storage", "Could not save the output folder.")
}

func addSettingsRoutes(mux *http.ServeMux, settings Settings) {
	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, settingsBody(settings))
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
			writeSettingsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, settingsBody(settings))
	})
	if prefs, ok := settings.(preferenceSettings); ok {
		mux.HandleFunc("PUT /api/settings/preferences", func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				MaxDownloads  *int           `json:"max_downloads"`
				DefaultPreset *domain.Preset `json:"default_preset"`
				SpeedLimit    *int           `json:"speed_limit_kbps"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "Send preferences as JSON.")
				return
			}
			var err error
			if input.MaxDownloads != nil {
				err = prefs.SetMaxDownloads(r.Context(), *input.MaxDownloads)
			}
			if err == nil && input.DefaultPreset != nil {
				err = prefs.SetDefaultPreset(r.Context(), *input.DefaultPreset)
			}
			if err == nil && input.SpeedLimit != nil {
				speed, ok := settings.(speedSettings)
				if !ok {
					writeError(w, http.StatusBadRequest, "invalid_preference", "This server has no speed limit setting.")
					return
				}
				err = speed.SetSpeedLimit(r.Context(), *input.SpeedLimit)
			}
			if errors.Is(err, settingspkg.ErrInvalidPreference) {
				writeError(w, http.StatusBadRequest, "invalid_preference", err.Error())
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "storage", "Could not save the setting.")
				return
			}
			writeJSON(w, http.StatusOK, settingsBody(settings))
		})
	}
	if cookies, ok := settings.(cookieSettings); ok {
		mux.HandleFunc("PUT /api/settings/cookies", func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Browser string `json:"browser"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "Send a browser name as JSON.")
				return
			}
			if err := cookies.SetCookiesBrowser(r.Context(), input.Browser); err != nil {
				if errors.Is(err, settingspkg.ErrInvalidBrowser) {
					writeError(w, http.StatusBadRequest, "invalid_browser", err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, "storage", "Could not save the setting.")
				return
			}
			writeJSON(w, http.StatusOK, settingsBody(settings))
		})
	}
	if subtitles, ok := settings.(subtitleSettings); ok {
		mux.HandleFunc("PUT /api/settings/subtitles", func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Mode string `json:"mode"`
				Lang string `json:"lang"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "Send a subtitle mode and language as JSON.")
				return
			}
			err := subtitles.SetSubtitles(r.Context(), input.Mode, input.Lang)
			if errors.Is(err, settingspkg.ErrInvalidPreference) {
				writeError(w, http.StatusBadRequest, "invalid_preference", "Choose a listed subtitle option and language.")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "storage", "Could not save the setting.")
				return
			}
			writeJSON(w, http.StatusOK, settingsBody(settings))
		})
	}
	if startup, ok := settings.(startupSettings); ok && startup.StartAtLoginSupported() {
		mux.HandleFunc("PUT /api/settings/startup", func(w http.ResponseWriter, r *http.Request) {
			var input struct {
				Enabled *bool `json:"enabled"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&input); err != nil || input.Enabled == nil {
				writeError(w, http.StatusBadRequest, "invalid_request", `Send {"enabled": true} or {"enabled": false}.`)
				return
			}
			if err := startup.SetStartAtLogin(*input.Enabled); err != nil {
				writeError(w, http.StatusInternalServerError, "startup", "Could not change whether YTGrab starts when you sign in.")
				return
			}
			writeJSON(w, http.StatusOK, settingsBody(settings))
		})
	}
	if first, ok := settings.(firstRunSettings); ok {
		mux.HandleFunc("POST /api/settings/use-default", func(w http.ResponseWriter, r *http.Request) {
			if err := first.UseDefault(r.Context()); err != nil {
				writeSettingsError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, settingsBody(settings))
		})
	}
	folders, ok := settings.(FolderPicker)
	if !ok {
		return
	}
	var picking sync.Mutex
	mux.HandleFunc("POST /api/settings/pick-folder", func(w http.ResponseWriter, r *http.Request) {
		if !picking.TryLock() {
			writeError(w, http.StatusConflict, "picker_busy", "A folder window is already open. Look for it on your desktop.")
			return
		}
		defer picking.Unlock()
		path, err := folders.Pick(r.Context(), initialFolder(settings))
		switch {
		case errors.Is(err, picker.ErrCancelled):
			body := settingsBody(settings)
			body["cancelled"] = true
			writeJSON(w, http.StatusOK, body)
		case errors.Is(err, picker.ErrUnavailable):
			writeError(w, http.StatusNotImplemented, "picker_unavailable", "No folder window is available here. Type the folder path instead.")
		case err != nil:
			if r.Context().Err() == nil {
				writeError(w, http.StatusInternalServerError, "picker_failed", "The folder window could not be opened. Type the folder path instead.")
			}
		default:
			if err := settings.SetDownloadsDir(r.Context(), path); err != nil {
				writeSettingsError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, settingsBody(settings))
		}
	})
}

// initialFolder opens the window at the current folder, or the user's home until it exists.
func initialFolder(settings Settings) string {
	if info, err := os.Stat(settings.DownloadsDir()); err == nil && info.IsDir() {
		return settings.DownloadsDir()
	}
	home, _ := os.UserHomeDir()
	return home
}

// RequireLoopbackHost rejects requests whose Host is not a loopback name. A DNS-rebinding
// page would otherwise reach this server as "same-origin" under its own host name.
func RequireLoopbackHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			writeError(w, http.StatusForbidden, "invalid_host", "Open YTGrab at http://127.0.0.1 on this computer.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
