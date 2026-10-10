package ytdlp

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/cooldown"
	"github.com/Sanoy24/ytgrab/internal/domain"
	"github.com/Sanoy24/ytgrab/internal/process"
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
	Protocol       string   `json:"protocol,omitempty"`
	TotalBitrate   *float64 `json:"tbr,omitempty"`
	FileSizeApprox *int64   `json:"filesize_approx"`
	Language       string   `json:"language,omitempty"`
	// LanguagePreference is 10 for a video's original audio and -1 for dubbed versions.
	LanguagePreference *int `json:"language_preference,omitempty"`
}

type Inspection struct {
	VideoID         string   `json:"video_id"`
	Title           string   `json:"title"`
	DurationSeconds *float64 `json:"duration_seconds"`
	Chapters        int      `json:"chapters"` // how many chapters the creator marked
	// Site is "" for YouTube or "x"; X posts also carry their preview image.
	Site      string `json:"site,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	// Videos lists every video of an X post that has several; empty otherwise.
	Videos []PostVideo `json:"videos,omitempty"`
	Video  []Format    `json:"video"`
	Audio  []Format    `json:"audio"`
}

// PostVideo is one video of an X post with several.
type PostVideo struct {
	Index           int      `json:"index"` // from 1, as in /video/N links
	VideoID         string   `json:"video_id"`
	URL             string   `json:"url"`
	DurationSeconds *float64 `json:"duration_seconds"`
	Thumbnail       string   `json:"thumbnail,omitempty"`
}

// rawVideo is the part of yt-dlp's JSON for one video that inspection reads.
type rawVideo struct {
	ID        string     `json:"id"`
	DisplayID string     `json:"display_id"`
	Thumbnail string     `json:"thumbnail"`
	Title     string     `json:"title"`
	Duration  *float64   `json:"duration"`
	Chapters  []struct{} `json:"chapters"`
	Formats   []Format   `json:"formats"`
}

type cachedInspection struct {
	result  Inspection
	expires time.Time
}

type Inspector struct {
	Config config.Config
	// CookiesBrowser, when set, returns the browser whose YouTube sign-in to use.
	CookiesBrowser func() string
	// Cooldown, when set, skips YouTube requests while YouTube is limiting this network
	// and starts a pause when a check is blocked.
	Cooldown *cooldown.Gate
	mu       sync.Mutex
	cache    map[string]cachedInspection
	titles   map[string]string // video ID -> title from recent playlist listings
	gate     chan struct{}
	last     time.Time
}

func NewInspector(cfg config.Config) *Inspector {
	return &Inspector{Config: cfg, cache: make(map[string]cachedInspection), titles: make(map[string]string), gate: make(chan struct{}, 1)}
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
	youtube := domain.SiteOf(url) == domain.SiteYouTube
	if youtube {
		if err := inspector.cooldownError(); err != nil { // a YouTube pause doesn't stop X
			return Inspection{}, err
		}
	}
	release, err := inspector.throttle(ctx)
	if err != nil {
		return Inspection{}, err
	}
	defer release()
	inspector.mu.Lock()
	if cached, ok := inspector.cache[videoID]; ok && time.Now().Before(cached.expires) {
		inspector.mu.Unlock()
		return cached.result, nil
	}
	inspector.mu.Unlock()
	result, err := inspector.extract(ctx, url, videoID)
	if youtube {
		inspector.noteResult(err)
	}
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
				short := *format.Height
				if format.Width != nil && *format.Width < short {
					short = *format.Width
				}
				label = fmt.Sprintf("Video · %dp", short)
			} else {
				label = "Video · " + strings.ToUpper(format.Ext)
			}
		}
		return domain.FormatSelection{Kind: kind, ID: id, Ext: format.Ext, Label: label}, true
	}
	return domain.FormatSelection{}, false
}

// cooldownError reports a pause in progress without contacting YouTube.
func (inspector *Inspector) cooldownError() error {
	if inspector.Cooldown == nil {
		return nil
	}
	until, paused := inspector.Cooldown.Until()
	if !paused {
		return nil
	}
	minutes := int(math.Ceil(time.Until(until).Minutes()))
	return &Error{Code: "blocked", Message: fmt.Sprintf("YouTube is limiting requests from this network. Try again in about %d min.", minutes)}
}

// noteResult starts a pause when YouTube blocked a check.
func (inspector *Inspector) noteResult(err error) {
	var toolError *Error
	if inspector.Cooldown != nil && errors.As(err, &toolError) && toolError.Code == "blocked" {
		inspector.Cooldown.Block()
	}
}

// throttle admits one YouTube metadata request at a time, at least two seconds apart,
// shared by format inspection and playlist listing.
func (inspector *Inspector) throttle(ctx context.Context) (func(), error) {
	select {
	case inspector.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	release := func() { <-inspector.gate }
	inspector.mu.Lock()
	wait := time.Until(inspector.last.Add(2 * time.Second))
	inspector.last = time.Now().Add(max(wait, 0))
	inspector.mu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			release()
			return nil, ctx.Err()
		}
	}
	return release, nil
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
	args := []string{"--ignore-config", "--no-playlist", "--skip-download", "--dump-json", "--", url}
	output, err := inspector.runJSON(ctx, path, args, 25*time.Second)
	if err != nil {
		return Inspection{}, err
	}
	result, err := parseInspection(output, videoID, domain.SiteOf(url))
	// Right after the bot check, an embedded player sometimes answers without formats;
	// asking again gets them.
	if failure, ok := err.(*Error); ok && failure.Code == "video_unavailable" && len(result.Video) == 0 && BotCheckActive() && ctx.Err() == nil {
		if output, retryErr := inspector.runJSON(ctx, path, args, 25*time.Second); retryErr == nil {
			return parseInspection(output, videoID, domain.SiteOf(url))
		}
	}
	return result, err
}

// runJSON runs yt-dlp for machine-readable metadata with a timeout and bounded output.
// The URL is always the last argument, after "--".
func (inspector *Inspector) runJSON(ctx context.Context, path string, args []string, timeout time.Duration) ([]byte, error) {
	if domain.SiteOf(args[len(args)-1]) != domain.SiteYouTube {
		return inspector.signedIn(ctx, path, args, timeout)
	}
	return withBotCheckFallback(ctx, func(clients string) ([]byte, error) {
		return inspector.signedIn(ctx, path, append(clientArgs(clients), args...), timeout)
	})
}

// signedIn runs yt-dlp with browser sign-in when it is on.
func (inspector *Inspector) signedIn(ctx context.Context, path string, args []string, timeout time.Duration) ([]byte, error) {
	if inspector.CookiesBrowser != nil {
		if cookies := cookieArgs(inspector.CookiesBrowser()); cookies != nil {
			output, err := inspector.runJSONArgs(ctx, path, append(cookies, args...), timeout)
			// As with downloads: unreadable browser sign-in shouldn't stop a look that may
			// not need it.
			if failure, ok := err.(*Error); ok && failure.Code == "cookies_failed" {
				return inspector.runJSONArgs(ctx, path, args, timeout)
			}
			return output, err
		}
	}
	return inspector.runJSONArgs(ctx, path, args, timeout)
}

// runJSONArgs runs yt-dlp once with the given arguments (sign-in already decided).
func (inspector *Inspector) runJSONArgs(ctx context.Context, path string, args []string, timeout time.Duration) ([]byte, error) {
	args = append(deps.RuntimeArgs(ctx, inspector.Config), args...)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.Command(path, args...)
	output := &limitedBuffer{limit: 16 << 20}
	diagnostic := &limitedBuffer{limit: 1 << 20}
	cmd.Stdout, cmd.Stderr = output, diagnostic
	stop, err := process.Start(runCtx, cmd)
	if err != nil {
		return nil, fmt.Errorf("start yt-dlp: %w", err)
	}
	err = cmd.Wait()
	stop()
	if runCtx.Err() != nil {
		return nil, runCtx.Err()
	}
	if err != nil {
		// The URL is always the last argument, after "--".
		return nil, classifyFor(domain.SiteOf(args[len(args)-1]), diagnostic.String())
	}
	return output.Bytes(), nil
}

func parseInspection(data []byte, expectedID, site string) (Inspection, error) {
	if site == domain.SiteX || site == domain.SiteInstagram {
		return parseMultiPost(data, expectedID, site)
	}
	var raw rawVideo
	if err := json.Unmarshal(data, &raw); err != nil {
		return Inspection{}, &Error{Code: "download_failed", Message: "yt-dlp returned invalid format information."}
	}
	if site != domain.SiteYouTube {
		return parsePostInspection(raw, expectedID, site)
	}
	if raw.ID != expectedID {
		return Inspection{}, &Error{Code: "video_unavailable", Message: "The inspected video did not match the requested link."}
	}
	result := Inspection{VideoID: raw.ID, Title: raw.Title, DurationSeconds: raw.Duration, Chapters: len(raw.Chapters), Video: []Format{}, Audio: []Format{}}
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

// parseXInspection lists an X post's MP4 files, which already contain sound, as the video
// choices. X lists no file sizes, so they are estimated from bitrate and length. Audio
// choices come from the page's presets (taken out of the best file).
func parseXInspection(videoID, title string, duration *float64, thumbnail string, formats []Format) (Inspection, error) {
	// X leaves HTML entities in post text ("R&amp;D"); decode them as the download does.
	result := Inspection{VideoID: videoID, Title: html.UnescapeString(title), DurationSeconds: duration, Site: domain.SiteX,
		Thumbnail: domain.SafeThumbnail(thumbnail), Video: []Format{}, Audio: []Format{}}
	// The MP4s' listed bitrate is nominal and runs about twice the real size; the
	// streaming copy at the same resolution reports a realistic video bitrate, plus audio.
	streamed := map[[2]int]float64{}
	audioKbps := 0.0
	for _, format := range formats {
		if format.TotalBitrate == nil || format.Protocol == "https" {
			continue
		}
		if format.Height != nil && format.Width != nil {
			streamed[[2]int{*format.Width, *format.Height}] = *format.TotalBitrate
		} else if format.VideoCodec == "none" && *format.TotalBitrate > audioKbps {
			audioKbps = *format.TotalBitrate
		}
	}
	for _, format := range formats {
		if format.Protocol != "https" || format.Height == nil || !(domain.FormatSelection{Kind: "video", ID: format.ID}).Valid() {
			continue
		}
		// yt-dlp fills filesize_approx from the nominal bitrate, so replace its guess.
		if format.FileSize == nil && duration != nil {
			kbps := 0.0
			if format.Width != nil {
				if video, ok := streamed[[2]int{*format.Width, *format.Height}]; ok {
					kbps = video + audioKbps
				}
			}
			if kbps == 0 && format.TotalBitrate != nil {
				kbps = *format.TotalBitrate
			}
			if kbps > 0 {
				approx := int64(kbps * *duration * 125) // kbit/s x s -> bytes
				format.FileSizeApprox = &approx
			}
		}
		result.Video = append(result.Video, format)
	}
	if len(result.Video) == 0 {
		return Inspection{}, &Error{Code: "video_unavailable", Message: "This post has no video."}
	}
	return result, nil
}

// parseMultiPost reads the videos of an X or Instagram post, which yt-dlp prints one JSON
// object each, and returns the one the link names: the first, or the Nth for a link to one
// of them. A post with several videos also lists them all, so the page can offer each.
func parseMultiPost(data []byte, expectedID, site string) (Inspection, error) {
	postID, index := domain.SplitItemID(site, expectedID)
	var entries []rawVideo
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var entry rawVideo
		if err := decoder.Decode(&entry); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return Inspection{}, &Error{Code: "download_failed", Message: "yt-dlp returned invalid format information."}
		}
		// X names every video after the post; a carousel's videos have codes of their own.
		if site == domain.SiteX && entry.DisplayID != postID {
			return Inspection{}, &Error{Code: "video_unavailable", Message: "The inspected post did not match the requested link."}
		}
		if len(entries) < domain.MaxCarouselItems {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 {
		return Inspection{}, &Error{Code: "video_unavailable", Message: "This post has no video."}
	}
	if index < 1 || index > len(entries) {
		return Inspection{}, &Error{Code: "video_unavailable", Message: fmt.Sprintf("This post has only %d video%s.", len(entries), map[bool]string{true: "", false: "s"}[len(entries) == 1])}
	}
	chosen := entries[index-1]
	var result Inspection
	var err error
	if site == domain.SiteX {
		result, err = parseXInspection(expectedID, chosen.Title, chosen.Duration, chosen.Thumbnail, chosen.Formats)
	} else {
		result, err = streamsInspection(chosen, expectedID, site)
	}
	if err != nil {
		return Inspection{}, err
	}
	itemID, itemURL := domain.XVideoID, domain.XVideoURL
	if site == domain.SiteInstagram {
		itemID, itemURL = domain.InstagramItemID, domain.InstagramItemURL
	}
	if len(entries) > 1 {
		for i, entry := range entries {
			result.Videos = append(result.Videos, PostVideo{
				Index:           i + 1,
				VideoID:         itemID(postID, i+1),
				URL:             itemURL(postID, i+1),
				DurationSeconds: entry.Duration,
				Thumbnail:       domain.SafeThumbnail(entry.Thumbnail),
			})
		}
	}
	return result, nil
}

// parsePostInspection reads a post from a site like Reddit, whose videos and audio come as
// separate streams like YouTube's but are matched by the post's ID (or a v.redd.it video's)
// and list no sizes, which are estimated from bitrate and length.
func parsePostInspection(raw rawVideo, expectedID, site string) (Inspection, error) {
	if raw.DisplayID != expectedID && raw.ID != expectedID {
		return Inspection{}, &Error{Code: "video_unavailable", Message: "The inspected post did not match the requested link."}
	}
	return streamsInspection(raw, expectedID, site)
}

// streamsInspection lists a post's separate video and audio streams, as Reddit and
// Instagram serve them, estimating sizes from bitrate and length when both are known.
func streamsInspection(raw rawVideo, expectedID, site string) (Inspection, error) {
	var unnamed []Format
	result := Inspection{VideoID: expectedID, Title: html.UnescapeString(raw.Title), DurationSeconds: raw.Duration, Site: site,
		Thumbnail: domain.SafeThumbnail(raw.Thumbnail), Video: []Format{}, Audio: []Format{}}
	for _, format := range raw.Formats {
		if !(domain.FormatSelection{Kind: "video", ID: format.ID}).Valid() {
			continue
		}
		if format.FileSize == nil && format.FileSizeApprox == nil && format.TotalBitrate != nil && raw.Duration != nil {
			approx := int64(*format.TotalBitrate * *raw.Duration * 125) // kbit/s x s -> bytes
			format.FileSizeApprox = &approx
		}
		if format.VideoCodec != "none" && format.VideoCodec != "" && format.AudioCodec == "none" {
			result.Video = append(result.Video, format)
		} else if format.AudioCodec != "none" && format.AudioCodec != "" && format.VideoCodec == "none" {
			result.Audio = append(result.Audio, format)
		} else if format.VideoCodec == "none" && format.AudioCodec == "" {
			unnamed = append(unnamed, format)
		}
	}
	// Vimeo's audio streams say they have no video but not which audio codec (AAC, in MP4);
	// use them when no stream names its codec, offered as the M4A files they are saved as,
	// the "high" one first.
	if len(result.Audio) == 0 {
		slices.SortStableFunc(unnamed, func(a, b Format) int {
			return cmp.Compare(btoi(!strings.Contains(a.ID, "-high")), btoi(!strings.Contains(b.ID, "-high")))
		})
		for _, format := range unnamed {
			format.Ext = "m4a"
			result.Audio = append(result.Audio, format)
		}
	}
	if len(result.Video) == 0 && len(result.Audio) == 0 {
		return Inspection{}, &Error{Code: "video_unavailable", Message: "This post has no video."}
	}
	return result, nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
