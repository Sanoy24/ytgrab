package ytdlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

// Listing is what a watched channel or playlist currently holds, newest first for a
// channel and in playlist order for a playlist.
type Listing struct {
	Title   string
	Entries []PlaylistEntry
}

// ListLatest lists up to limit entries of a channel's uploads or a playlist, sharing the
// inspector's request throttle and the pause when YouTube limits the network. The URL must
// come from domain.NewWatch.
func (inspector *Inspector) ListLatest(ctx context.Context, kind, url string, limit int) (Listing, error) {
	if err := inspector.cooldownError(); err != nil {
		return Listing{}, err
	}
	path, err := deps.Find(inspector.Config, "yt-dlp")
	if err != nil {
		return Listing{}, &Error{Code: "dependency_missing", Message: "Install yt-dlp and add it to PATH or the tools directory."}
	}
	release, err := inspector.throttle(ctx)
	if err != nil {
		return Listing{}, err
	}
	defer release()
	args := []string{
		"--ignore-config", "--flat-playlist", "--dump-single-json", "--sleep-requests", "0.5",
		"--playlist-items", fmt.Sprintf("1:%d", limit), "--", url,
	}
	output, err := inspector.runJSON(ctx, path, args, 90*time.Second)
	inspector.noteResult(err)
	var toolError *Error
	if errors.As(err, &toolError) && toolError.Code == "video_unavailable" {
		return Listing{}, &Error{Code: "video_unavailable", Message: "This channel or playlist is unavailable or private."}
	}
	if err != nil {
		return Listing{}, err
	}
	listing, err := parseListing(output, kind)
	if err == nil {
		inspector.rememberTitles(Playlist{Entries: listing.Entries})
	}
	return listing, err
}

func parseListing(data []byte, kind string) (Listing, error) {
	var raw struct {
		Title   string `json:"title"`
		Channel string `json:"channel"`
		Entries []struct {
			ID         string   `json:"id"`
			Title      string   `json:"title"`
			Duration   *float64 `json:"duration"`
			LiveStatus string   `json:"live_status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Listing{}, &Error{Code: "download_failed", Message: "yt-dlp returned invalid channel information."}
	}
	listing := Listing{Title: raw.Title, Entries: []PlaylistEntry{}}
	if kind == "channel" {
		// A channel's uploads are titled "Name - Videos"; the channel's name reads better.
		listing.Title = raw.Channel
		if listing.Title == "" {
			listing.Title = strings.TrimSuffix(raw.Title, " - Videos")
		}
	}
	for _, entry := range raw.Entries {
		if !domain.ValidVideoID(entry.ID) || unavailableTitles[entry.Title] {
			continue
		}
		listing.Entries = append(listing.Entries, PlaylistEntry{VideoID: entry.ID, Title: entry.Title, DurationSeconds: entry.Duration, LiveStatus: entry.LiveStatus})
	}
	return listing, nil
}
