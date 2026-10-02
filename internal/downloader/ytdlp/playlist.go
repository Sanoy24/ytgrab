package ytdlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

type PlaylistEntry struct {
	VideoID         string   `json:"video_id"`
	Title           string   `json:"title"`
	DurationSeconds *float64 `json:"duration_seconds"`
}

// Playlist is one page of up to domain.PlaylistPageSize entries, starting at the 1-based
// position Start. Total is YouTube's count when known. Next is the position of the
// following page, or nil on the last page; Truncated reports the same.
type Playlist struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Entries     []PlaylistEntry `json:"entries"`
	Total       *int            `json:"total"`
	Start       int             `json:"start"`
	Next        *int            `json:"next"`
	Truncated   bool            `json:"truncated"`
	Unavailable int             `json:"unavailable"`
}

// ListPlaylist reads a playlist's entries without resolving each video, sharing the
// inspector's request throttle.
func (inspector *Inspector) ListPlaylist(ctx context.Context, rawURL string, start int) (Playlist, error) {
	if start < 1 {
		start = 1
	}
	url, listID, err := domain.ParsePlaylistURL(rawURL)
	if err != nil {
		return Playlist{}, err
	}
	if err := inspector.cooldownError(); err != nil {
		return Playlist{}, err
	}
	path, err := deps.Find(inspector.Config, "yt-dlp")
	if err != nil {
		return Playlist{}, &Error{Code: "dependency_missing", Message: "Install yt-dlp and add it to PATH or the tools directory."}
	}
	release, err := inspector.throttle(ctx)
	if err != nil {
		return Playlist{}, err
	}
	defer release()
	output, err := inspector.runJSON(ctx, path, playlistArgs(url, start), 45*time.Second)
	inspector.noteResult(err)
	var toolError *Error
	if errors.As(err, &toolError) && toolError.Code == "video_unavailable" {
		return Playlist{}, &Error{Code: "video_unavailable", Message: "This playlist is unavailable or private."}
	}
	if err != nil {
		return Playlist{}, err
	}
	list, err := parsePlaylist(output, listID, start)
	if err == nil {
		inspector.rememberTitles(list)
	}
	return list, err
}

// rememberTitles keeps listed titles so jobs created from a playlist show them while
// queued. The map is bounded; it is display data only.
func (inspector *Inspector) rememberTitles(list Playlist) {
	inspector.mu.Lock()
	defer inspector.mu.Unlock()
	if len(inspector.titles) > 1000 {
		clear(inspector.titles)
	}
	for _, entry := range list.Entries {
		inspector.titles[entry.VideoID] = entry.Title
	}
}

// CachedTitle returns a title known from a recent inspection or playlist listing.
func (inspector *Inspector) CachedTitle(videoID string) string {
	inspector.mu.Lock()
	defer inspector.mu.Unlock()
	if cached, ok := inspector.cache[videoID]; ok {
		return cached.result.Title
	}
	return inspector.titles[videoID]
}

// playlistArgs asks for one page plus one entry, so a following page can be detected.
func playlistArgs(url string, start int) []string {
	return []string{
		"--ignore-config", "--flat-playlist", "--dump-single-json", "--sleep-requests", "0.5",
		"--playlist-items", fmt.Sprintf("%d:%d", start, start+domain.PlaylistPageSize),
		"--", url,
	}
}

func parsePlaylist(data []byte, expectedID string, start int) (Playlist, error) {
	var raw struct {
		ID      string `json:"id"`
		Title   string `json:"title"`
		Count   *int   `json:"playlist_count"`
		Entries []struct {
			ID       string   `json:"id"`
			Title    string   `json:"title"`
			Duration *float64 `json:"duration"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Playlist{}, &Error{Code: "download_failed", Message: "yt-dlp returned invalid playlist information."}
	}
	if raw.ID != expectedID {
		return Playlist{}, &Error{Code: "video_unavailable", Message: "The listed playlist did not match the requested link."}
	}
	list := Playlist{ID: raw.ID, Title: raw.Title, Entries: []PlaylistEntry{}, Total: raw.Count, Start: start}
	for i, entry := range raw.Entries {
		if i >= domain.PlaylistPageSize {
			list.Truncated = true
			break
		}
		// Flat listings keep private and deleted videos as placeholders.
		if !domain.ValidVideoID(entry.ID) || entry.Title == "[Private video]" || entry.Title == "[Deleted video]" {
			list.Unavailable++
			continue
		}
		list.Entries = append(list.Entries, PlaylistEntry{VideoID: entry.ID, Title: entry.Title, DurationSeconds: entry.Duration})
	}
	if raw.Count != nil && *raw.Count >= start+domain.PlaylistPageSize {
		list.Truncated = true
	}
	if list.Truncated {
		next := start + domain.PlaylistPageSize
		list.Next = &next
	}
	if len(list.Entries) == 0 {
		return Playlist{}, &Error{Code: "video_unavailable", Message: "This playlist has no downloadable videos."}
	}
	return list, nil
}
