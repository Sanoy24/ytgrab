package ytdlp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

var ErrInvalidQuery = errors.New("Type between 2 and 200 characters to search YouTube.")

// SearchResult is one video found by a search.
type SearchResult struct {
	VideoID         string   `json:"video_id"`
	Title           string   `json:"title"`
	Channel         string   `json:"channel"`
	DurationSeconds *float64 `json:"duration_seconds"`
}

// searchResults is how many videos one search lists.
const searchResults = 12

// Search lists YouTube videos matching words, sharing the inspector's throttle and the
// pause when YouTube limits the network. The words go to yt-dlp's ytsearch as one argument
// after "--", so they can never become an option.
func (inspector *Inspector) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.Join(strings.Fields(query), " ")
	if n := utf8.RuneCountInString(query); n < 2 || n > 200 {
		return nil, ErrInvalidQuery
	}
	if err := inspector.cooldownError(); err != nil {
		return nil, err
	}
	path, err := deps.Find(inspector.Config, "yt-dlp")
	if err != nil {
		return nil, &Error{Code: "dependency_missing", Message: "Install yt-dlp and add it to PATH or the tools directory."}
	}
	release, err := inspector.throttle(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	args := []string{"--ignore-config", "--flat-playlist", "--dump-single-json", "--", "ytsearch12:" + query}
	output, err := inspector.runJSON(ctx, path, args, 45*time.Second)
	inspector.noteResult(err)
	if err != nil {
		return nil, err
	}
	results, err := parseSearch(output)
	if err == nil {
		list := Playlist{}
		for _, r := range results {
			list.Entries = append(list.Entries, PlaylistEntry{VideoID: r.VideoID, Title: r.Title})
		}
		inspector.rememberTitles(list)
	}
	return results, err
}

func parseSearch(data []byte) ([]SearchResult, error) {
	var raw struct {
		Entries []struct {
			ID         string   `json:"id"`
			Title      string   `json:"title"`
			Channel    string   `json:"channel"`
			Uploader   string   `json:"uploader"`
			Duration   *float64 `json:"duration"`
			LiveStatus string   `json:"live_status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &Error{Code: "download_failed", Message: "yt-dlp returned invalid search results."}
	}
	results := []SearchResult{}
	for _, entry := range raw.Entries {
		if !domain.ValidVideoID(entry.ID) || entry.LiveStatus == "is_live" || entry.LiveStatus == "is_upcoming" {
			continue
		}
		channel := entry.Channel
		if channel == "" {
			channel = entry.Uploader
		}
		results = append(results, SearchResult{VideoID: entry.ID, Title: entry.Title, Channel: channel, DurationSeconds: entry.Duration})
		if len(results) == searchResults {
			break
		}
	}
	return results, nil
}
