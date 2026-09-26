package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"ytgrab/internal/app/deps"
	"ytgrab/internal/config"
	"ytgrab/internal/domain"
	"ytgrab/internal/process"
)

type Format struct {
	ID             string   `json:"format_id"`
	Ext            string   `json:"ext"`
	Height         *int     `json:"height,omitempty"`
	Width          *int     `json:"width,omitempty"`
	FPS            *float64 `json:"fps,omitempty"`
	VideoCodec     string   `json:"vcodec,omitempty"`
	AudioCodec     string   `json:"acodec,omitempty"`
	AudioBitrate   *float64 `json:"abr,omitempty"`
	FileSize       *int64   `json:"filesize"`
	FileSizeApprox *int64   `json:"filesize_approx"`
	Language       string   `json:"language,omitempty"`
}

type Inspection struct {
	VideoID         string   `json:"video_id"`
	Title           string   `json:"title"`
	DurationSeconds *float64 `json:"duration_seconds"`
	Video           []Format `json:"video"`
	Audio           []Format `json:"audio"`
}

type cachedInspection struct {
	result  Inspection
	expires time.Time
}

type Inspector struct {
	Config config.Config
	mu     sync.Mutex
	cache  map[string]cachedInspection
	gate   chan struct{}
	last   time.Time
}

func NewInspector(cfg config.Config) *Inspector {
	return &Inspector{Config: cfg, cache: make(map[string]cachedInspection), gate: make(chan struct{}, 1)}
}

func (inspector *Inspector) Inspect(ctx context.Context, rawURL string) (Inspection, error) {
	url, videoID, err := domain.ParseVideoURL(rawURL)
	if err != nil {
		return Inspection{}, err
	}
	inspector.mu.Lock()
	if cached, ok := inspector.cache[videoID]; ok && time.Now().Before(cached.expires) {
		inspector.mu.Unlock()
		return cached.result, nil
	}
	inspector.mu.Unlock()
	select {
	case inspector.gate <- struct{}{}:
	case <-ctx.Done():
		return Inspection{}, ctx.Err()
	}
	defer func() { <-inspector.gate }()
	inspector.mu.Lock()
	if cached, ok := inspector.cache[videoID]; ok && time.Now().Before(cached.expires) {
		inspector.mu.Unlock()
		return cached.result, nil
	}
	wait := time.Until(inspector.last.Add(2 * time.Second))
	inspector.last = time.Now().Add(max(wait, 0))
	inspector.mu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return Inspection{}, ctx.Err()
		}
	}
	result, err := inspector.extract(ctx, url, videoID)
	if err != nil {
		return Inspection{}, err
	}
	inspector.mu.Lock()
	if len(inspector.cache) >= 64 {
		for id := range inspector.cache {
			delete(inspector.cache, id)
			break
		}
	}
	inspector.cache[videoID] = cachedInspection{result: result, expires: time.Now().Add(10 * time.Minute)}
	inspector.mu.Unlock()
	return result, nil
}

func (inspector *Inspector) Select(videoID string, kind string, id string) (domain.FormatSelection, bool) {
	inspector.mu.Lock()
	defer inspector.mu.Unlock()
	cached, ok := inspector.cache[videoID]
	if !ok || time.Now().After(cached.expires) {
		return domain.FormatSelection{}, false
	}
	formats := cached.result.Video
	if kind == "audio" {
		formats = cached.result.Audio
	} else if kind != "video" {
		return domain.FormatSelection{}, false
	}
	for _, format := range formats {
		if format.ID != id {
			continue
		}
		label := "Audio · " + strings.ToUpper(format.Ext)
		if format.AudioBitrate != nil {
			label += fmt.Sprintf(" %.0f kbps", *format.AudioBitrate)
		}
		if kind == "video" {
			if format.Height != nil {
				label = fmt.Sprintf("Video · %dp", *format.Height)
			} else {
				label = "Video · " + strings.ToUpper(format.Ext)
			}
		}
		return domain.FormatSelection{Kind: kind, ID: id, Ext: format.Ext, Label: label}, true
	}
	return domain.FormatSelection{}, false
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *limitedBuffer) Write(p []byte) (int, error) {
	if buffer.Len()+len(p) > buffer.limit {
		return 0, errors.New("inspection output exceeded limit")
	}
	return buffer.Buffer.Write(p)
}

func (inspector *Inspector) extract(ctx context.Context, url string, videoID string) (Inspection, error) {
	path, err := deps.Find(inspector.Config, "yt-dlp")
	if err != nil {
		return Inspection{}, &Error{Code: "dependency_missing", Message: "Install yt-dlp and add it to PATH or the tools directory."}
	}
	args := []string{"--ignore-config", "--no-playlist", "--skip-download", "--dump-json"}
	if _, err := deps.Find(inspector.Config, "deno"); err != nil {
		if _, err := deps.Find(inspector.Config, "node"); err == nil {
			args = append(args, "--js-runtimes", "node")
		}
	}
	args = append(args, "--", url)
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd := exec.Command(path, args...)
	output := &limitedBuffer{limit: 16 << 20}
	diagnostic := &limitedBuffer{limit: 1 << 20}
	cmd.Stdout, cmd.Stderr = output, diagnostic
	stop, err := process.Start(runCtx, cmd)
	if err != nil {
		return Inspection{}, fmt.Errorf("start inspection: %w", err)
	}
	err = cmd.Wait()
	stop()
	if runCtx.Err() != nil {
		return Inspection{}, runCtx.Err()
	}
	if err != nil {
		return Inspection{}, classifyFailure(diagnostic.String())
	}
	return parseInspection(output.Bytes(), videoID)
}

func parseInspection(data []byte, expectedID string) (Inspection, error) {
	var raw struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Duration *float64 `json:"duration"`
		Formats  []Format `json:"formats"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Inspection{}, &Error{Code: "download_failed", Message: "yt-dlp returned invalid format information."}
	}
	if raw.ID != expectedID {
		return Inspection{}, &Error{Code: "video_unavailable", Message: "The inspected video did not match the requested link."}
	}
	result := Inspection{VideoID: raw.ID, Title: raw.Title, DurationSeconds: raw.Duration, Video: []Format{}, Audio: []Format{}}
	for _, format := range raw.Formats {
		if !(domain.FormatSelection{Kind: "video", ID: format.ID}).Valid() {
			continue
		}
		if format.VideoCodec != "none" && format.VideoCodec != "" && format.AudioCodec == "none" {
			result.Video = append(result.Video, format)
		} else if format.AudioCodec != "none" && format.AudioCodec != "" && format.VideoCodec == "none" {
			result.Audio = append(result.Audio, format)
		}
	}
	if len(result.Video) == 0 && len(result.Audio) == 0 {
		return Inspection{}, &Error{Code: "video_unavailable", Message: "No downloadable formats were found for this video."}
	}
	return result, nil
}
