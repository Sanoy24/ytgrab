// Package library deletes finished downloads' files, with what was saved beside them,
// for the Library's "Delete file too" and for watches that keep only their newest videos.
// It only ever touches files YTGrab recognizably wrote.
package library

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

// Owned returns the job's saved file if it is still there and is recognizably the
// file YTGrab wrote for this video (its name carries "[video-id]").
func Owned(job domain.Job) (string, bool) {
	if job.State != domain.Completed || job.OutputPath == nil || !filepath.IsAbs(*job.OutputPath) {
		return "", false
	}
	path := *job.OutputPath
	return path, strings.Contains(filepath.Base(path), "["+job.VideoID+"]")
}

// chapterFile matches the files "Split into chapters" writes: "01 Intro.m4a".
var chapterFile = regexp.MustCompile(`^[0-9]{2,3} .*\.[A-Za-z0-9]{2,5}$`)

// Remove deletes a finished download's file and, when it was split into chapters,
// the chapter files in the folder named like it. Only chapter files are removed from that
// folder, which is then removed if nothing else is left in it. A file that is already gone
// is not an error.
func Remove(job domain.Job, path string) (removed bool, err error) {
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
