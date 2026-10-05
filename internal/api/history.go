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
	removeSidecars(path)
	removeEmptyShow(filepath.Dir(path))
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

// sidecarFile matches what yt-dlp writes beside a download, after its name: metadata,
// the thumbnail, and subtitles ("Title [id] 720p.en.srt").
var sidecarFile = regexp.MustCompile(`^(-thumb\.(jpg|jpeg|webp|png)|\.(nfo|info\.json|description|jpg|jpeg|webp|png|([A-Za-z0-9_-]{1,20}\.)?(srt|vtt|ass|lrc)))$`)

// removeSidecars deletes the files yt-dlp wrote beside a deleted download: only those named
// exactly like it, followed by one of sidecarFile's endings.
func removeSidecars(path string) {
	dir, base := filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type().IsRegular() && strings.HasPrefix(name, base) && sidecarFile.MatchString(name[len(base):]) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// seasonFolder matches the folders of the media-server file-name style: "Season 2005".
var seasonFolder = regexp.MustCompile(`^Season [0-9]{4}$`)

// removeEmptyShow tidies up after the last episode of a show is deleted: an empty season
// folder goes, and so does the show folder when all that is left is the tvshow.nfo YTGrab
// wrote. Anything else in either folder keeps it.
func removeEmptyShow(season string) {
	if !seasonFolder.MatchString(filepath.Base(season)) || os.Remove(season) != nil {
		return // not a season folder, or not empty
	}
	show := filepath.Dir(season)
	entries, err := os.ReadDir(show)
	if err != nil || len(entries) != 1 || entries[0].Name() != "tvshow.nfo" {
		return
	}
	nfo := filepath.Join(show, "tvshow.nfo")
	if data, err := os.ReadFile(nfo); err == nil && strings.Contains(string(data), "saved by YTGrab.") {
		_ = os.Remove(nfo)
		_ = os.Remove(show)
	}
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

	// A yt-dlp download archive ("youtube jNQXAC9IVRw" per line) of finished downloads, so
	// yt-dlp scripts and other tools skip what YTGrab already has. Only YouTube and Vimeo:
	// for X, Reddit, and Instagram YTGrab keeps the post's ID, not the one yt-dlp records.
	mux.HandleFunc("GET /api/library/archive", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := store.List(r.Context(), 500)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not read the library.")
			return
		}
		seen := map[string]bool{}
		var lines strings.Builder
		for _, job := range jobs {
			extractor := map[string]string{domain.SiteYouTube: "youtube", domain.SiteVimeo: "vimeo"}[job.Site]
			line := extractor + " " + job.VideoID
			if job.State != domain.Completed || extractor == "" || seen[line] {
				continue
			}
			seen[line] = true
			lines.WriteString(line + "\n")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="ytgrab-archive.txt"`)
		_, _ = w.Write([]byte(lines.String()))
	})

	// The Library's file sizes, read on demand rather than with every job list: one look
	// at each finished download's file, which also tells moved or deleted files apart.
	mux.HandleFunc("GET /api/library/files", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := store.List(r.Context(), 500)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "storage", "Could not read the library.")
			return
		}
		files := map[string]libraryFile{}
		for _, job := range jobs {
			if job.State != domain.Completed || job.OutputPath == nil {
				continue
			}
			if info, err := os.Stat(*job.OutputPath); err == nil && info.Mode().IsRegular() {
				files[job.ID] = libraryFile{Bytes: info.Size()}
			} else {
				files[job.ID] = libraryFile{Missing: true}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": files})
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

// libraryFile is a finished download's file as it is now: its size, or missing when it was
// moved or deleted outside YTGrab.
type libraryFile struct {
	Bytes   int64 `json:"bytes"`
	Missing bool  `json:"missing,omitempty"`
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
