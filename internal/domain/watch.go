package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var ErrInvalidWatchURL = errors.New("Paste a link to a YouTube channel (like youtube.com/@name) or a playlist.")

// A watch follows a channel's uploads or a playlist and downloads videos added to it.
type Watch struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // "channel" or "playlist"
	URL    string `json:"url"`  // canonical; what is listed
	Title  string `json:"title"`
	Preset Preset `json:"preset"`
	// Folder saves its downloads into a folder named after the channel or playlist.
	Folder bool `json:"folder"`
	Paused bool `json:"paused"`
	// Filters: skip videos shorter than MinMinutes (0: any length), and when Keywords is
	// set, download only titles containing one of its comma-separated words.
	MinMinutes int    `json:"min_minutes"`
	Keywords   string `json:"keywords,omitempty"`
	// IntervalHours is how often it is checked: 1, 6 (the default), or 24.
	IntervalHours int        `json:"interval_hours"`
	CreatedAt     time.Time  `json:"created_at"`
	LastChecked   *time.Time `json:"last_checked"`
	LastError     string     `json:"last_error,omitempty"`
	LastNew       int        `json:"last_new"`   // videos queued by the last check
	Downloaded    int        `json:"downloaded"` // videos queued since the watch was added
}

// Channel names: @handles, channel IDs, and the older /c/ and /user/ names.
var (
	handlePattern    = regexp.MustCompile(`^@[A-Za-z0-9._-]{3,30}$`)
	channelIDPattern = regexp.MustCompile(`^UC[A-Za-z0-9_-]{22}$`)
	legacyName       = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// NewWatch validates a channel or playlist link and returns a watch for it. Channel links
// are rewritten to the channel's uploads ("/videos"); playlist links to the playlist page.
func NewWatch(raw string, preset Preset) (Watch, error) {
	if !preset.Valid() {
		return Watch{}, ErrInvalidPreset
	}
	kind, canonical, err := parseWatchURL(raw)
	if err != nil {
		return Watch{}, err
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return Watch{}, err
	}
	return Watch{ID: "watch_" + hex.EncodeToString(bytes), Kind: kind, URL: canonical, Preset: preset, CreatedAt: time.Now().UTC()}, nil
}

func parseWatchURL(raw string) (kind, canonical string, err error) {
	value := strings.TrimSpace(raw)
	if value != "" && !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, perr := url.Parse(value)
	if perr != nil || len(value) > 2048 || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Port() != "" {
		return "", "", ErrInvalidWatchURL
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
	default:
		return "", "", ErrInvalidWatchURL
	}
	if parsed.Query().Get("list") != "" {
		playlist, _, perr := ParsePlaylistURL(value)
		if perr != nil {
			return "", "", perr // includes the "Mixes can't be downloaded" message
		}
		return "playlist", playlist, nil
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	const base = "https://www.youtube.com/"
	switch {
	case len(parts) >= 1 && handlePattern.MatchString(parts[0]):
		return "channel", base + parts[0] + "/videos", nil
	case len(parts) >= 2 && parts[0] == "channel" && channelIDPattern.MatchString(parts[1]):
		return "channel", base + "channel/" + parts[1] + "/videos", nil
	case len(parts) >= 2 && (parts[0] == "c" || parts[0] == "user") && legacyName.MatchString(parts[1]):
		return "channel", base + parts[0] + "/" + parts[1] + "/videos", nil
	}
	return "", "", ErrInvalidWatchURL
}

var ErrInvalidWatchOptions = errors.New("Choose a minimum length up to 600 minutes, keywords up to 200 characters, and a check every 1, 6, or 24 hours.")

// Interval is how often the watch is checked.
func (w Watch) Interval() time.Duration {
	if w.IntervalHours == 1 || w.IntervalHours == 24 {
		return time.Duration(w.IntervalHours) * time.Hour
	}
	return 6 * time.Hour
}

// ValidWatchOptions checks a watch's filters and interval.
func ValidWatchOptions(minMinutes int, keywords string, intervalHours int) bool {
	return minMinutes >= 0 && minMinutes <= 600 && len(keywords) <= 200 &&
		(intervalHours == 0 || intervalHours == 1 || intervalHours == 6 || intervalHours == 24)
}

// Wants reports whether a listed video passes the watch's filters. A video whose length
// isn't listed passes the length filter.
func (w Watch) Wants(title string, durationSeconds *float64) bool {
	if w.MinMinutes > 0 && durationSeconds != nil && *durationSeconds < float64(w.MinMinutes*60) {
		return false
	}
	words := strings.FieldsFunc(strings.ToLower(w.Keywords), func(r rune) bool { return r == ',' })
	if len(words) == 0 {
		return true
	}
	lower := strings.ToLower(title)
	for _, word := range words {
		if word = strings.TrimSpace(word); word != "" && strings.Contains(lower, word) {
			return true
		}
	}
	return false
}
