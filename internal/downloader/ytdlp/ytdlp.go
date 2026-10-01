package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/process"
)

const progressPrefix = "YTGRAB_PROGRESS:"
const processingPrefix = "YTGRAB_PROCESS:"
const titlePrefix = "YTGRAB_TITLE:"
const pathPrefix = "YTGRAB_PATH:"

type Event struct {
	State      domain.State
	Progress   *domain.Progress
	Title      string
	OutputPath string
}

type Result struct {
	Title      string
	OutputPath string
}

type Error struct {
	Code    string
	Message string
}

func (err *Error) Error() string { return err.Message }

type Downloader struct {
	Config       config.Config
	DownloadsDir func() string
}

func (downloader Downloader) Download(ctx context.Context, job domain.Job, onEvent func(Event) error) (Result, error) {
	if job.Format != nil && !job.Format.Valid() {
		return Result{}, &Error{Code: "invalid_format", Message: "Choose a format from a recent inspection."}
	}
	cfg := downloader.Config
	if downloader.DownloadsDir != nil {
		cfg.DownloadsDir = downloader.DownloadsDir()
	}
	path, err := deps.Find(cfg, "yt-dlp")
	if err != nil {
		return Result{}, &Error{Code: "dependency_missing", Message: "Install yt-dlp and add it to PATH or the tools directory."}
	}
	if err := os.MkdirAll(cfg.DownloadsDir, 0700); err != nil {
		return Result{}, fmt.Errorf("create download directory: %w", err)
	}
	args := buildArgs(job, cfg)
	if ffmpeg, err := deps.Find(cfg, "ffmpeg"); err == nil {
		args = append([]string{"--ffmpeg-location", filepath.Dir(ffmpeg)}, args...)
	}
	if _, err := deps.Find(cfg, "deno"); err != nil {
		if _, err := deps.Find(cfg, "node"); err == nil {
			args = append([]string{"--js-runtimes", "node"}, args...)
		}
	}
	cmd := exec.Command(path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop, err := process.Start(runCtx, cmd)
	if err != nil {
		return Result{}, fmt.Errorf("start yt-dlp: %w", err)
	}

	var result Result
	var callbackErr error
	var stderrTail string
	var eventMu sync.Mutex
	consumeEvent := func(line string) bool {
		event, recognized := parseEvent(line)
		if !recognized {
			return false
		}
		eventMu.Lock()
		defer eventMu.Unlock()
		if event.Title != "" {
			result.Title = event.Title
		}
		if event.OutputPath != "" {
			result.OutputPath = event.OutputPath
		}
		if callbackErr == nil && onEvent != nil {
			if err := onEvent(event); err != nil {
				callbackErr = err
				cancel()
			}
		}
		return true
	}
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		scanLines(stdout, func(line string) { consumeEvent(line) })
	}()
	go func() {
		defer group.Done()
		scanLines(stderr, func(line string) {
			if consumeEvent(line) {
				return
			}
			stderrTail += line + "\n"
			if len(stderrTail) > 4096 {
				stderrTail = stderrTail[len(stderrTail)-4096:]
			}
		})
	}()
	group.Wait()
	waitErr := cmd.Wait()
	stop()
	if callbackErr != nil {
		return Result{}, callbackErr
	}
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if waitErr != nil {
		return Result{}, classifyFailure(stderrTail)
	}
	if result.OutputPath == "" {
		return Result{}, &Error{Code: "download_failed", Message: "yt-dlp finished without reporting an output file."}
	}
	confirmed, err := confirmOutput(cfg.DownloadsDir, result.OutputPath)
	if err != nil {
		return Result{}, &Error{Code: "download_failed", Message: "The completed output file could not be confirmed."}
	}
	result.OutputPath = confirmed
	return result, nil
}

func buildArgs(job domain.Job, cfg config.Config) []string {
	args := []string{
		"--ignore-config", "--no-playlist", "--no-simulate", "--newline", "--progress",
		"--progress-template", "download:" + progressPrefix + `{"progress":%(progress)j,"vcodec":%(info.vcodec)j}`,
		"--progress-template", "postprocess:" + processingPrefix + "%(progress)j",
		"--print", "before_dl:" + titlePrefix + "%(title)j",
		"--print", "after_move:" + pathPrefix + "%(filepath)j",
		"--concurrent-fragments", "4",
		// A short pause between YouTube requests makes a download look less like a burst.
		"--sleep-requests", "0.5",
		"-P", cfg.DownloadsDir,
		"-o", outputTemplate(job),
	}
	if job.Format != nil {
		if job.Format.Kind == "video" {
			args = append(args, "-f", videoSelector(*job.Format), "--merge-output-format", "mp4/webm/mkv")
		} else {
			args = append(args, "-f", job.Format.ID)
		}
		return append(args, "--", job.URL)
	}
	if job.Preset == nil {
		return append(args, "--", job.URL)
	}
	switch *job.Preset {
	case domain.VideoBest:
		args = append(args, "-f", "bv*+ba/b", "--merge-output-format", "mp4/mkv")
	case domain.Video1080:
		args = append(args, "-f", "bv*[height<=1080]+ba/b[height<=1080]", "--merge-output-format", "mp4/mkv")
	case domain.Video720:
		args = append(args, "-f", "bv*[height<=720]+ba/b[height<=720]", "--merge-output-format", "mp4/mkv")
	case domain.AudioM4A:
		args = append(args, "-f", "ba[ext=m4a]")
	case domain.AudioMP3:
		args = append(args, "-f", "ba", "-x", "--audio-format", "mp3", "--audio-quality", "0")
	}
	return append(args, "--", job.URL)
}

// outputTemplate names files by quality so different picks of one video never collide;
// otherwise yt-dlp would report an earlier quality as "already downloaded".
func outputTemplate(job domain.Job) string {
	const base = "%(title).150B [%(id)s]"
	switch {
	case job.Format != nil && job.Format.Kind == "video":
		return base + " %(height)sp.%(ext)s"
	case job.Format != nil:
		return base + " %(abr).0fk.%(ext)s"
	case job.Preset == nil:
		return base + ".%(ext)s"
	}
	switch *job.Preset {
	case domain.VideoBest, domain.Video1080, domain.Video720:
		return base + " %(height)sp.%(ext)s"
	case domain.AudioM4A:
		return base + " %(abr).0fk.%(ext)s"
	default:
		return base + ".%(ext)s"
	}
}

// videoSelector pairs the chosen video with audio in the same container family so the
// merge needs no re-encode and stays MP4 (H.264/AV1 + AAC) or WebM (VP9 + Opus), falling
// back to any audio, then to the video alone.
func videoSelector(format domain.FormatSelection) string {
	id := format.ID
	switch format.Ext {
	case "mp4":
		return id + "+ba[ext=m4a]/" + id + "+ba/" + id
	case "webm":
		return id + "+ba[ext=webm]/" + id + "+ba/" + id
	default:
		return id + "+ba/" + id
	}
}

func parseEvent(line string) (Event, bool) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, progressPrefix):
		var wrapper struct {
			Progress struct {
				DownloadedBytes    int64    `json:"downloaded_bytes"`
				TotalBytes         *int64   `json:"total_bytes"`
				TotalBytesEstimate *int64   `json:"total_bytes_estimate"`
				Speed              *float64 `json:"speed"`
				ETA                *int64   `json:"eta"`
			} `json:"progress"`
			VideoCodec *string `json:"vcodec"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, progressPrefix)), &wrapper); err != nil {
			return Event{}, false
		}
		raw := wrapper.Progress
		stream := ""
		if wrapper.VideoCodec != nil {
			stream = "video"
			if *wrapper.VideoCodec == "none" {
				stream = "audio"
			}
		}
		total := raw.TotalBytes
		if total == nil {
			total = raw.TotalBytesEstimate
		}
		return Event{State: domain.Downloading, Progress: &domain.Progress{DownloadedBytes: raw.DownloadedBytes, TotalBytes: total, SpeedBPS: raw.Speed, ETASeconds: raw.ETA, Stream: stream}}, true
	case strings.HasPrefix(line, processingPrefix):
		return Event{State: domain.Processing}, true
	case strings.HasPrefix(line, titlePrefix):
		var title string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, titlePrefix)), &title); err != nil {
			return Event{}, false
		}
		return Event{Title: title}, true
	case strings.HasPrefix(line, pathPrefix):
		var path string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, pathPrefix)), &path); err != nil {
			return Event{}, false
		}
		return Event{OutputPath: path}, true
	default:
		return Event{}, false
	}
}

func scanLines(reader io.Reader, consume func(string)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		consume(scanner.Text())
	}
}

func confirmOutput(directory string, reportedPath string) (string, error) {
	path := reportedPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(directory, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(directory, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", errors.New("output path escaped download directory")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("output path is not a regular file")
	}
	return path, nil
}

func classifyFailure(stderr string) error {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "http error 429"), strings.Contains(lower, "sign in to confirm you're not a bot"), strings.Contains(lower, "sign in to confirm you’re not a bot"), strings.Contains(lower, "too many requests"):
		return &Error{Code: "blocked", Message: "YouTube is limiting requests from this network. Wait a while, then retry."}
	case strings.Contains(lower, "private video"), strings.Contains(lower, "video unavailable"), strings.Contains(lower, "not available"), strings.Contains(lower, "does not exist"):
		return &Error{Code: "video_unavailable", Message: "This video is unavailable or private."}
	case strings.Contains(lower, "ffmpeg not found"), strings.Contains(lower, "ffprobe not found"):
		return &Error{Code: "dependency_missing", Message: "Install ffmpeg and ffprobe, then retry."}
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "connection"), strings.Contains(lower, "http error 5"):
		return &Error{Code: "network", Message: "The download failed because of a network error. Retry shortly."}
	default:
		return &Error{Code: "download_failed", Message: "yt-dlp could not download this video. Check that yt-dlp is up to date and retry."}
	}
}
