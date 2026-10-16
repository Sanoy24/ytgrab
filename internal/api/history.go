package api

import (
	"bufio"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/library"
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

// ownedOutput and removeOutput are the library's checks, as the routes below use them.
func ownedOutput(job domain.Job) (string, bool) { return library.Owned(job) }

func removeOutput(job domain.Job, path string) (bool, error) { return library.Remove(job, path) }

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

	// The saved file itself, for "Save to phone", or with ?play=1 to play in the browser.
	// Range requests let a phone start playing before the whole file arrives.
	mux.HandleFunc("GET /api/jobs/{id}/file", func(w http.ResponseWriter, r *http.Request) {
		job, err := store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		path, owned := ownedOutput(job)
		if !owned {
			writeError(w, http.StatusConflict, "not_completed", "Only finished downloads can be saved.")
			return
		}
		file, err := os.Open(path)
		if err != nil {
			writeError(w, http.StatusNotFound, "file_missing", "The file was moved or deleted.")
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			writeError(w, http.StatusNotFound, "file_missing", "The file was moved or deleted.")
			return
		}
		disposition := "attachment"
		if r.URL.Query().Get("play") == "1" {
			disposition = "inline"
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filepath.Base(path)}))
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
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

	// Imports a yt-dlp download archive ("youtube jNQXAC9IVRw" per line): those videos count
	// as already downloaded, so playlist reviews leave them unticked and watches skip them.
	if archive, ok := store.(archiveStore); ok {
		mux.HandleFunc("POST /api/library/archive", func(w http.ResponseWriter, r *http.Request) {
			videos, skipped, err := parseArchive(http.MaxBytesReader(w, r.Body, 8<<20))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_archive", "That file is too large or unreadable.")
				return
			}
			if len(videos) == 0 {
				writeError(w, http.StatusBadRequest, "invalid_archive", "No YouTube or Vimeo videos found. A yt-dlp archive has one \"youtube <id>\" line per video.")
				return
			}
			added, err := archive.AddArchived(r.Context(), videos)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "storage", "Could not save the archive.")
				return
			}
			writeJSON(w, http.StatusOK, map[string]int{"added": added, "already": len(videos) - added, "skipped": skipped})
		})
	}

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

// haveStore knows which videos the user already has (SQLite store).
type haveStore interface {
	Have(context.Context, []string) (map[string]bool, error)
}

// archiveStore remembers videos from an imported yt-dlp archive.
type archiveStore interface {
	AddArchived(context.Context, []sqlitestore.ArchivedVideo) (int, error)
}

// archiveSites are the yt-dlp extractors whose archive lines YTGrab can use.
var archiveSites = map[string]string{"youtube": domain.SiteYouTube, "vimeo": domain.SiteVimeo}

// parseArchive reads a yt-dlp download archive. Lines for other sites, and IDs that don't
// look like the site's, are counted as skipped.
func parseArchive(body io.Reader) ([]sqlitestore.ArchivedVideo, int, error) {
	var videos []sqlitestore.ArchivedVideo
	seen := map[string]bool{}
	skipped := 0
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		site, known := archiveSites[strings.ToLower(fields[0])]
		if len(fields) != 2 || !known || !domain.ValidWatchVideoID(site, fields[1]) {
			skipped++
			continue
		}
		if key := site + " " + fields[1]; !seen[key] {
			seen[key] = true
			videos = append(videos, sqlitestore.ArchivedVideo{Site: site, VideoID: fields[1]})
		}
	}
	return videos, skipped, scanner.Err()
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
