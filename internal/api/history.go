package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// chapterFile matches the files "Split into chapters" writes: "01 Intro.m4a".
var chapterFile = regexp.MustCompile(`^[0-9]{2,3} .*\.[A-Za-z0-9]{2,5}$`)

// removeOutput deletes a finished download's file and, when it was split into chapters,
// the chapter files in the folder named like it. Only chapter files are removed from that
// folder, which is then removed if nothing else is left in it. A file that is already gone
// is not an error.
func removeOutput(job domain.Job, path string) (removed bool, err error) {
	if err := os.Remove(path); err == nil {
		removed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if !job.SplitChapters {
		return removed, nil
	}
	folder := strings.TrimSuffix(path, filepath.Ext(path))
	entries, err := os.ReadDir(folder)
	if err != nil {
		return removed, nil // no chapter folder
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && chapterFile.MatchString(entry.Name()) {
			if err := os.Remove(filepath.Join(folder, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return removed, err
			}
		}
	}
	_ = os.Remove(folder) // only succeeds when empty
	return removed, nil
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
			if _, err := removeOutput(job, path); err != nil {
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
		if r.URL.Query().Get("delete_files") == "true" {
			result, err := clearWithFiles(r.Context(), store, deleter)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "storage", "Could not clear the history.")
				return
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		removed, err := deleter.DeleteFinished(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not clear the history.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"removed": removed})
	})
}

// clearResult reports a clear that also deleted files. Kept counts finished downloads
// whose file couldn't be deleted (open in another program, say); they stay in the list
// so nothing is lost track of.
type clearResult struct {
	Removed      int `json:"removed"`
	FilesDeleted int `json:"files_deleted"`
	Kept         int `json:"kept"`
}

// clearWithFiles removes every finished, failed, and cancelled job, deleting each finished
// download's file first. Only files that pass the same check as deleting one download are
// touched; an entry whose file was already gone is still removed.
func clearWithFiles(ctx context.Context, store JobStore, deleter jobDeleter) (clearResult, error) {
	var result clearResult
	kept := map[string]bool{}
	for {
		jobs, err := store.List(ctx, 500) // the most the store returns at once
		if err != nil {
			return result, err
		}
		progressed := false
		for _, job := range jobs {
			if job.State.Active() || kept[job.ID] {
				continue
			}
			if path, owned := ownedOutput(job); owned {
				removed, err := removeOutput(job, path)
				if err != nil {
					kept[job.ID] = true
					result.Kept++
					continue
				}
				if removed {
					result.FilesDeleted++
				}
			}
			if err := deleter.Delete(ctx, job.ID); err == nil {
				result.Removed++
				progressed = true
			} else if !errors.Is(err, sqlitestore.ErrConflict) && !errors.Is(err, sqlitestore.ErrNotFound) {
				return result, err
			}
		}
		// Stop once a pass removes nothing; a full page means older jobs may remain.
		if !progressed || len(jobs) < 500 {
			return result, nil
		}
	}
}
