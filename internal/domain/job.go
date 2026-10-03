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

type State string

const (
	Queued      State = "queued"
	Inspecting  State = "inspecting"
	Downloading State = "downloading"
	Processing  State = "processing"
	Completed   State = "completed"
	Failed      State = "failed"
	Cancelled   State = "cancelled"
	// Paused stays in the queue (and blocks a second copy of the video) but isn't started
	// until it is resumed; the partial file is kept, so it continues where it stopped.
	Paused State = "paused"
)

type Preset string

const (
	VideoBest Preset = "video-best"
	Video1080 Preset = "video-1080"
	Video720  Preset = "video-720"
	AudioM4A  Preset = "audio-m4a"
	AudioMP3  Preset = "audio-mp3"
	AudioOpus Preset = "audio-opus"
	AudioFLAC Preset = "audio-flac"
	AudioWAV  Preset = "audio-wav"
)

var ErrInvalidURL = errors.New("enter a valid single YouTube video URL")
var ErrInvalidPreset = errors.New("choose a supported download preset")
var ErrInvalidFormat = errors.New("Choose a format from a recent inspection.")
var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
var formatIDPattern = regexp.MustCompile(`^[0-9A-Za-z_-]{1,32}$`)

type FormatSelection struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	// Ext is the inspected container; the downloader uses it to pair compatible audio.
	Ext   string `json:"ext,omitempty"`
	Label string `json:"label"`
}

func (format FormatSelection) Valid() bool {
	return (format.Kind == "video" || format.Kind == "audio") && formatIDPattern.MatchString(format.ID)
}

type Progress struct {
	DownloadedBytes int64    `json:"downloaded_bytes"`
	TotalBytes      *int64   `json:"total_bytes"`
	SpeedBPS        *float64 `json:"speed_bps"`
	ETASeconds      *int64   `json:"eta_seconds"`
	// Stream is "video" or "audio" while yt-dlp fetches one part of a merged download.
	Stream string `json:"stream,omitempty"`
}

type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Job struct {
	ID         string           `json:"id"`
	URL        string           `json:"url"`
	VideoID    string           `json:"video_id"`
	Title      *string          `json:"title"`
	Preset     *Preset          `json:"preset"`
	Format     *FormatSelection `json:"format"`
	State      State            `json:"state"`
	Attempt    int              `json:"attempt"`
	Progress   *Progress        `json:"progress"`
	OutputPath *string          `json:"output_path"`
	// Folder is an optional subfolder of the download folder (see SafeFolderName).
	Folder string `json:"folder,omitempty"`
	// Priority orders the queue: higher starts first, then older. "Move to top" raises it.
	Priority int64 `json:"priority,omitempty"`
	// SplitChapters also saves each chapter as its own file, in a folder next to the video.
	SplitChapters bool `json:"split_chapters,omitempty"`
	// Section, when set, downloads only that part of the video.
	Section   *Section  `json:"section,omitempty"`
	Error     *JobError `json:"error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewJob(rawURL string, preset Preset) (Job, error) {
	if !preset.Valid() {
		return Job{}, ErrInvalidPreset
	}
	job, err := newBaseJob(rawURL)
	if err != nil {
		return Job{}, err
	}
	job.Preset = &preset
	return job, nil
}

func NewFormatJob(rawURL string, format FormatSelection) (Job, error) {
	if !format.Valid() || format.Label == "" {
		return Job{}, ErrInvalidFormat
	}
	job, err := newBaseJob(rawURL)
	if err != nil {
		return Job{}, err
	}
	job.Format = &format
	return job, nil
}

func newBaseJob(rawURL string) (Job, error) {
	url, videoID, err := ParseVideoURL(rawURL)
	if err != nil {
		return Job{}, err
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return Job{}, err
	}
	now := time.Now().UTC()
	return Job{
		ID:        "job_" + hex.EncodeToString(bytes),
		URL:       url,
		VideoID:   videoID,
		State:     Queued,
		Attempt:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (preset Preset) Valid() bool {
	switch preset {
	case VideoBest, Video1080, Video720, AudioM4A, AudioMP3, AudioOpus, AudioFLAC, AudioWAV:
		return true
	default:
		return false
	}
}

func ParseVideoURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 2048 {
		return "", "", ErrInvalidURL
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Port() != "" {
		return "", "", ErrInvalidURL
	}
	host := strings.ToLower(parsed.Hostname())
	var videoID string
	switch host {
	case "youtu.be":
		videoID = strings.Trim(parsed.Path, "/")
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) == 1 && parts[0] == "watch" {
			videoID = parsed.Query().Get("v")
		} else if len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "live" || parts[0] == "embed") {
			videoID = parts[1]
		}
	default:
		return "", "", ErrInvalidURL
	}
	if !videoIDPattern.MatchString(videoID) {
		return "", "", ErrInvalidURL
	}
	return parsed.String(), videoID, nil
}

// Playlists are listed PlaylistPageSize entries at a time, and one confirmation creates
// at most MaxPlaylistJobs jobs.
const (
	PlaylistPageSize = 50
	MaxPlaylistJobs  = 200
)

var ErrInvalidPlaylistURL = errors.New("Enter a YouTube playlist link.")
var ErrMixPlaylist = errors.New("YouTube Mixes are generated endlessly and can't be downloaded as a playlist. Open a single video instead.")
var playlistIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{10,64}$`)

// ParsePlaylistURL accepts a YouTube link carrying a list parameter and returns the
// canonical playlist URL, so yt-dlp lists the playlist rather than one video.
func ParsePlaylistURL(raw string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 2048 {
		return "", "", ErrInvalidPlaylistURL
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Port() != "" {
		return "", "", ErrInvalidPlaylistURL
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com":
	default:
		return "", "", ErrInvalidPlaylistURL
	}
	listID := parsed.Query().Get("list")
	if !playlistIDPattern.MatchString(listID) {
		return "", "", ErrInvalidPlaylistURL
	}
	if strings.HasPrefix(listID, "RD") {
		return "", "", ErrMixPlaylist
	}
	return "https://www.youtube.com/playlist?list=" + listID, listID, nil
}

// VideoURL builds the canonical watch URL for a validated video ID.
func VideoURL(videoID string) string {
	return "https://www.youtube.com/watch?v=" + videoID
}

func ValidVideoID(videoID string) bool {
	return videoIDPattern.MatchString(videoID)
}

func (state State) Active() bool {
	switch state {
	case Queued, Inspecting, Downloading, Processing, Paused:
		return true
	default:
		return false
	}
}

func CanTransition(from State, to State) bool {
	if from == to {
		return true // Progress or metadata update.
	}
	switch from {
	case Queued:
		return to == Inspecting || to == Downloading || to == Failed || to == Cancelled || to == Paused
	case Inspecting:
		return to == Downloading || to == Failed || to == Cancelled || to == Queued || to == Paused
	case Downloading:
		return to == Processing || to == Completed || to == Failed || to == Cancelled || to == Queued || to == Paused
	case Processing:
		return to == Completed || to == Failed || to == Cancelled || to == Queued || to == Paused
	case Paused:
		return to == Queued || to == Cancelled
	case Failed, Cancelled:
		return to == Queued
	default:
		return false
	}
}

// Requeue returns a running job to the queue as a new attempt, for example after
// YouTube limited the network. Use Transition for other state changes.
func (job *Job) Requeue() error {
	if !job.State.Active() || job.State == Queued {
		return errors.New("only a running job can be requeued")
	}
	job.State = Queued
	job.Attempt++
	job.Progress = nil
	job.Error = nil
	job.OutputPath = nil
	job.UpdatedAt = time.Now().UTC()
	return nil
}

func (job *Job) Transition(to State) error {
	if !CanTransition(job.State, to) || (job.State.Active() && job.State != Queued && job.State != Paused && to == Queued) {
		return errors.New("invalid job state transition")
	}
	if (job.State == Failed || job.State == Cancelled) && to == Queued {
		job.Attempt++
		job.Progress = nil
		job.Error = nil
		job.OutputPath = nil
	}
	job.State = to
	job.UpdatedAt = time.Now().UTC()
	return nil
}
