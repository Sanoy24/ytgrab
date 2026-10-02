package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/reveal"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// showFile opens the file manager at a file; replaced in tests.
var showFile = reveal.Show

// openFile opens a file with its default app; replaced in tests.
var openFile = reveal.OpenFile

// jobDeleter is implemented by stores that can remove finished jobs.
type jobDeleter interface {
	Delete(context.Context, string) error
	DeleteFinished(context.Context) (int, error)
}

// ownedOutput returns the job's saved file if it is still there and is recognizably the
// file YTGrab wrote for this video (its name carries "[video-id]").
func ownedOutput(job domain.Job) (string, bool) {
	if job.State != domain.Completed || job.OutputPath == nil || !filepath.IsAbs(*job.OutputPath) {
		return "", false
	}
	path := *job.OutputPath
	return path, strings.Contains(filepath.Base(path), "["+job.VideoID+"]")
}

func addHistoryRoutes(mux *http.ServeMux, store JobStore) {
	mux.HandleFunc("POST /api/jobs/{id}/reveal", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if job.State != domain.Completed || job.OutputPath == nil {
			writeError(w, http.StatusConflict, "not_completed", "Only finished downloads can be shown in their folder.")
			return
		}
		if info, err := os.Stat(*job.OutputPath); err != nil || !info.Mode().IsRegular() {
			writeError(w, http.StatusNotFound, "file_missing", "The file was moved or deleted.")
			return
		}
		if err := showFile(*job.OutputPath); err != nil {
			writeError(w, http.StatusInternalServerError, "reveal_failed", "The folder could not be opened.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/jobs/{id}/open", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		// Only the file this download saved: the same check as deleting it.
		path, owned := ownedOutput(job)
		if !owned {
			writeError(w, http.StatusConflict, "not_completed", "Only finished downloads can be opened.")
			return
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			writeError(w, http.StatusNotFound, "file_missing", "The file was moved or deleted.")
			return
		}
		if err := openFile(path); err != nil {
			writeError(w, http.StatusInternalServerError, "open_failed", "The file could not be opened.")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	deleter, ok := store.(jobDeleter)
	if !ok {
		return
	}
	mux.HandleFunc("DELETE /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if job.State.Active() {
			writeError(w, http.StatusConflict, "invalid_state", "Cancel the download before removing it.")
			return
		}
		if r.URL.Query().Get("delete_file") == "true" {
			path, owned := ownedOutput(job)
			if !owned {
				writeError(w, http.StatusBadRequest, "not_deletable", "This file can't be deleted from YTGrab. Remove it from the list instead.")
				return
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				writeError(w, http.StatusConflict, "delete_failed", "The file could not be deleted. It may be open in another program.")
				return
			}
		}
		if err := deleter.Delete(r.Context(), job.ID); err != nil {
			if errors.Is(err, sqlitestore.ErrConflict) {
				writeError(w, http.StatusConflict, "invalid_state", "The download changed; refresh and try again.")
				return
			}
			writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/history/clear", func(w http.ResponseWriter, r *http.Request) {
		removed, err := deleter.DeleteFinished(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not clear the history.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
	})
}
