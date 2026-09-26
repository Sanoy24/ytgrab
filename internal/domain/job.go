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
)

type Preset string

const (
	VideoBest Preset = "video-best"
	Video1080 Preset = "video-1080"
	Video720  Preset = "video-720"
	AudioM4A  Preset = "audio-m4a"
	AudioMP3  Preset = "audio-mp3"
)

var ErrInvalidURL = errors.New("enter a valid single YouTube video URL")
var ErrInvalidPreset = errors.New("choose a supported download preset")
var videoIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

type Progress struct {
	DownloadedBytes int64    `json:"downloaded_bytes"`
	TotalBytes      *int64   `json:"total_bytes"`
	SpeedBPS        *float64 `json:"speed_bps"`
	ETASeconds      *int64   `json:"eta_seconds"`
}

type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Job struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	VideoID    string    `json:"video_id"`
	Title      *string   `json:"title"`
	Preset     Preset    `json:"preset"`
	State      State     `json:"state"`
	Attempt    int       `json:"attempt"`
	Progress   *Progress `json:"progress"`
	OutputPath *string   `json:"output_path"`
	Error      *JobError `json:"error"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func NewJob(rawURL string, preset Preset) (Job, error) {
	if !preset.Valid() {
		return Job{}, ErrInvalidPreset
	}
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
		Preset:    preset,
		State:     Queued,
		Attempt:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (preset Preset) Valid() bool {
	switch preset {
	case VideoBest, Video1080, Video720, AudioM4A, AudioMP3:
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

func (state State) Active() bool {
	switch state {
	case Queued, Inspecting, Downloading, Processing:
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
		return to == Inspecting || to == Downloading || to == Failed || to == Cancelled
	case Inspecting:
		return to == Downloading || to == Failed || to == Cancelled
	case Downloading:
		return to == Processing || to == Completed || to == Failed || to == Cancelled
	case Processing:
		return to == Completed || to == Failed || to == Cancelled
	case Failed, Cancelled:
		return to == Queued
	default:
		return false
	}
}

func (job *Job) Transition(to State) error {
	if !CanTransition(job.State, to) {
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
