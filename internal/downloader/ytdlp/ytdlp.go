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
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/diskspace"
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
	// Detail is yt-dlp's own error lines, kept for diagnostic reports.
	Detail string
}

func (err *Error) Error() string { return err.Message }

type Downloader struct {
	Config         config.Config
	DownloadsDir   func() string
	CookiesBrowser func() string
	Subtitles      func() (mode, lang string)
	SpeedLimit     func() int // kB/s, 0 for none
	FileNames      func() string
	SponsorBlock   func() string
	NormalizeAudio func() bool
}

func (downloader Downloader) Download(ctx context.Context, job domain.Job, onEvent func(Event) error) (Result, error) {
	if job.Format != nil && !job.Format.Valid() {
		return Result{}, &Error{Code: "invalid_format", Message: "Choose a format from a recent inspection."}
	}
	cfg := downloader.Config
	if downloader.DownloadsDir != nil {
		cfg.DownloadsDir = downloader.DownloadsDir()
	}
	if job.Folder != "" {
		// Checked again here: only a name SafeFolderName produced is used as a subfolder.
		if job.Folder != domain.SafeFolderName(job.Folder) {
			return Result{}, &Error{Code: "invalid_folder", Message: "The folder for this download isn't valid. Remove it and add the video again."}
		}
		cfg.DownloadsDir = filepath.Join(cfg.DownloadsDir, job.Folder)
	}
	if downloader.CookiesBrowser != nil {
		cfg.CookiesBrowser = downloader.CookiesBrowser()
	}
	if downloader.Subtitles != nil {
		cfg.SubtitlesMode, cfg.SubtitlesLang = downloader.Subtitles()
	}
	if downloader.SpeedLimit != nil {
		cfg.SpeedLimitKBps = downloader.SpeedLimit()
	}
	if downloader.FileNames != nil {
		cfg.FileNames = downloader.FileNames()
	}
	if downloader.SponsorBlock != nil {
		cfg.SponsorBlock = downloader.SponsorBlock()
	}
	if downloader.NormalizeAudio != nil {
		cfg.NormalizeAudio = downloader.NormalizeAudio()
	}
	if err := checkFreeSpace(cfg.DownloadsDir); err != nil {
		return Result{}, err
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
	args = append(deps.RuntimeArgs(ctx, cfg), args...)
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
	var filter stateFilter
	consumeEvent := func(line string) bool {
		event, recognized := parseEvent(line)
		if !recognized {
			return false
		}
		event = filter.apply(event)
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
		err := classifyFor(job.Site, stderrTail)
		if failure, ok := err.(*Error); ok {
			failure.Detail = errorDetail(stderrTail)
		}
		if failure, ok := err.(*Error); ok && (failure.Code == "drm_protected" || failure.Code == "video_unavailable") {
			removeThumbnails(cfg.DownloadsDir, job.VideoID) // a retry won't use them
		}
		return Result{}, err
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
		"-o", outputTemplate(job, cfg.FileNames),
	}
	args = append(args, cookieArgs(cfg.CookiesBrowser)...)
	switch job.Site {
	case domain.SiteYouTube:
		args = append(args, sponsorBlockArgs(cfg.SponsorBlock)...) // SponsorBlock only knows YouTube
	case domain.SiteX:
		args = append(args, xTitleCleanup...)
	}
	if cfg.NormalizeAudio && convertsAudio(job) {
		// EBU R128 loudness normalization while the audio is converted anyway, at the level
		// streaming services use; M4A and Opus stay untouched copies.
		args = append(args, "--postprocessor-args", "ExtractAudio:-af loudnorm=I=-16:TP=-1.5:LRA=11")
	}
	if cfg.SpeedLimitKBps > 0 {
		args = append(args, "--limit-rate", strconv.Itoa(cfg.SpeedLimitKBps)+"K")
	}
	// Title, artist, date, and chapters go into the file; cover art only into formats that
	// hold it (M4A, MP3, Opus, FLAC), since a failed embed would fail the whole job.
	args = append(args, "--embed-metadata", "--embed-chapters")
	if embedsCoverArt(job) {
		args = append(args, "--embed-thumbnail", "--convert-thumbnails", "jpg")
	}
	if isVideo(job) && job.Site == domain.SiteYouTube {
		args = append(args, subtitleArgs(cfg.SubtitlesMode, cfg.SubtitlesLang)...)
	}
	if job.SplitChapters && job.Section == nil && job.Site == domain.SiteYouTube {
		// The full file is kept; the chapters go into a folder named like it.
		folder := strings.TrimSuffix(outputTemplate(job, cfg.FileNames), ".%(ext)s")
		args = append(args, "--split-chapters", "-o", "chapter:"+folder+"/%(section_number)02d %(section_title).100B.%(ext)s")
	}
	if job.Section != nil && job.Section.Valid() {
		// Exact cuts re-encode just the start and end; without them the clip would begin at
		// the nearest keyframe, often seconds early.
		args = append(args, "--download-sections", "*"+seconds(job.Section.Start)+"-"+seconds(job.Section.End), "--force-keyframes-at-cuts")
	}
	if job.Site == domain.SiteX {
		// yt-dlp reads every video of a post from the post's link; --playlist-items picks one.
		post, index := domain.XPost(job.URL)
		args = append(args, xFormatArgs(job)...)
		return append(args, "--playlist-items", strconv.Itoa(index), "--", post)
	}
	target := job.URL
	if job.Site == domain.SiteInstagram {
		// Like X: yt-dlp reads a carousel's videos from the post's link; pick one.
		post, index := domain.PostItem(job.URL)
		target = post
		args = append(args, "--playlist-items", strconv.Itoa(index))
	}
	finish := func(args []string) []string {
		if (job.Site == domain.SiteInstagram || job.Site == domain.SiteVimeo) && isVideo(job) {
			// Instagram's VP9 comes in MP4 with AAC audio, and Vimeo's audio doesn't name its
			// codec. yt-dlp won't pair either in MP4 by itself and falls back to MKV, but MP4
			// holds them fine and plays more widely. The later option wins.
			args = append(args, "--merge-output-format", "mp4")
		}
		return append(args, "--", target)
	}
	if job.Format != nil {
		if job.Format.Kind == "video" {
			args = append(args, "-f", videoSelector(*job.Format), "--merge-output-format", "mp4/webm/mkv")
		} else {
			args = append(args, "-f", job.Format.ID)
			if job.Site != domain.SiteYouTube && (job.Format.Ext == "m4a" || job.Format.Ext == "mp4") {
				args = append(args, "-x", "--audio-format", "m4a") // Vimeo's audio comes in MP4
			}
		}
		return finish(args)
	}
	if job.Preset == nil {
		return finish(args)
	}
	switch *job.Preset {
	case domain.VideoBest:
		args = append(args, "-f", "bv*+ba/b", "--merge-output-format", "mp4/mkv")
	case domain.Video1080:
		args = append(args, "-f", "bv*[height<=1080]+ba/b[height<=1080]", "--merge-output-format", "mp4/mkv")
	case domain.Video720:
		args = append(args, "-f", "bv*[height<=720]+ba/b[height<=720]", "--merge-output-format", "mp4/mkv")
	case domain.AudioM4A:
		if job.Site == domain.SiteYouTube {
			args = append(args, "-f", "ba[ext=m4a]")
		} else {
			// Vimeo's AAC comes in MP4; repackaging it as M4A doesn't re-encode.
			args = append(args, "-f", "ba[ext=m4a]/ba", "-x", "--audio-format", "m4a")
		}
	case domain.AudioMP3:
		args = append(args, "-f", "ba", "-x", "--audio-format", "mp3", "--audio-quality", "0")
	case domain.AudioOpus:
		// YouTube's own audio is usually Opus already: this repackages it without re-encoding.
		args = append(args, "-f", "ba[acodec=opus]/ba", "-x", "--audio-format", "opus")
	case domain.AudioFLAC:
		args = append(args, "-f", "ba", "-x", "--audio-format", "flac")
	case domain.AudioWAV:
		args = append(args, "-f", "ba", "-x", "--audio-format", "wav")
	}
	return finish(args)
}

// xTitleCleanup decodes the HTML entities X leaves in post text, so file names and tags
// read "R&D" rather than "R&amp;D".
var xTitleCleanup = []string{
	"--replace-in-metadata", "title", "&amp;", "&",
	"--replace-in-metadata", "title", "&lt;", "<",
	"--replace-in-metadata", "title", "&gt;", ">",
	"--replace-in-metadata", "title", "&quot;", `"`,
	"--replace-in-metadata", "title", "&#39;", "'",
}

// xFormatArgs picks X formats. X offers MP4 files that already contain sound ("b"), so
// no merging is needed; quality limits use the shorter side ("res"), since many X videos
// are portrait (1080x1920 is "1080p"). Audio is taken out of the best file.
func xFormatArgs(job domain.Job) []string {
	if job.Format != nil {
		if job.Format.Kind == "video" {
			return []string{"-f", job.Format.ID}
		}
		return []string{"-f", "ba/b", "-x", "--audio-format", "m4a"}
	}
	if job.Preset == nil {
		return []string{"-f", "b"}
	}
	switch *job.Preset {
	case domain.Video1080:
		return []string{"-f", "b", "-S", "res:1080"}
	case domain.Video720:
		return []string{"-f", "b", "-S", "res:720"}
	case domain.AudioM4A:
		return []string{"-f", "ba/b", "-x", "--audio-format", "m4a"}
	case domain.AudioMP3:
		return []string{"-f", "ba/b", "-x", "--audio-format", "mp3", "--audio-quality", "0"}
	case domain.AudioOpus:
		return []string{"-f", "ba/b", "-x", "--audio-format", "opus"}
	case domain.AudioFLAC:
		return []string{"-f", "ba/b", "-x", "--audio-format", "flac"}
	case domain.AudioWAV:
		return []string{"-f", "ba/b", "-x", "--audio-format", "wav"}
	default:
		return []string{"-f", "b"}
	}
}

// convertsAudio reports presets whose audio FFmpeg re-encodes (MP3, FLAC, WAV).
func convertsAudio(job domain.Job) bool {
	if job.Preset == nil || job.Format != nil {
		return false
	}
	switch *job.Preset {
	case domain.AudioMP3, domain.AudioFLAC, domain.AudioWAV:
		return true
	}
	return false
}

func isVideo(job domain.Job) bool {
	if job.Format != nil {
		return job.Format.Kind == "video"
	}
	return job.Preset != nil && (*job.Preset == domain.VideoBest || *job.Preset == domain.Video1080 || *job.Preset == domain.Video720)
}

// sponsorCategories are the SponsorBlock segments YTGrab marks or removes: paid sponsors,
// self-promotion, and "like and subscribe" reminders. Intros and recaps are left alone.
const sponsorCategories = "sponsor,selfpromo,interaction"

// sponsorBlockArgs looks up the video's segments in the SponsorBlock database and marks
// them as chapters or cuts them out.
func sponsorBlockArgs(mode string) []string {
	switch mode {
	case "mark":
		return []string{"--sponsorblock-mark", sponsorCategories}
	case "remove":
		return []string{"--sponsorblock-remove", sponsorCategories}
	default:
		return nil
	}
}

// languageCode matches the short codes in settings.SubtitleLanguages.
var languageCode = regexp.MustCompile(`^[a-z]{2,3}$`)

// subtitleArgs asks for subtitles in exactly lang: the creator's, or YouTube's automatic
// ones when there are none. That is one request; a pattern like "en.*" would also fetch
// every machine translation ("English from German", ...) and quickly hit YouTube's
// limits. They are embedded in the video or saved beside it as .srt.
func subtitleArgs(mode, lang string) []string {
	if (mode != "embed" && mode != "file") || !languageCode.MatchString(lang) {
		return nil
	}
	args := []string{"--write-subs", "--write-auto-subs", "--sub-langs", lang}
	if mode == "embed" {
		// With --write-subs given, yt-dlp would otherwise keep the file after embedding it.
		return append(args, "--embed-subs", "--compat-options", "no-keep-subs")
	}
	return append(args, "--convert-subs", "srt")
}

func embedsCoverArt(job domain.Job) bool {
	// Reddit's preview images claim one image type and are served as another, so converting
	// them fails, which fails the whole download; a video still isn't album art anyway.
	if job.Site == domain.SiteReddit {
		return false
	}
	if job.Format != nil {
		return job.Format.Kind == "audio" && job.Format.Ext == "m4a"
	}
	// WAV can't hold cover art; yt-dlp would fail the job trying.
	switch {
	case job.Preset == nil:
		return false
	case *job.Preset == domain.AudioM4A, *job.Preset == domain.AudioMP3, *job.Preset == domain.AudioOpus, *job.Preset == domain.AudioFLAC:
		return true
	}
	return false
}

// minFreeSpace is the least free space a download may start with; below it, a download
// would likely fail part-way and leave a partial file behind.
const minFreeSpace = 256_000_000

// freeSpace reports free bytes on a folder's drive; replaced in tests.
var freeSpace = diskspace.Free

func checkFreeSpace(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil // the download itself reports folder problems
	}
	free, err := freeSpace(dir)
	if err != nil || free >= minFreeSpace {
		return nil
	}
	return &Error{Code: "disk_full", Message: fmt.Sprintf("Only %s free on the drive with your download folder. Free up space or choose another folder, then Retry.", diskspace.Format(free))}
}

// browserName matches the plain lowercase names in settings.CookieBrowsers.
var browserName = regexp.MustCompile(`^[a-z]{2,16}$`)

// cookieArgs lets yt-dlp use the browser's YouTube sign-in when the user turned it on.
// YTGrab never reads or stores the cookies itself.
func cookieArgs(browser string) []string {
	if !browserName.MatchString(browser) {
		return nil
	}
	return []string{"--cookies-from-browser", browser}
}

// outputTemplate names files by quality so different picks of one video never collide;
// otherwise yt-dlp would report an earlier quality as "already downloaded".
func outputTemplate(job domain.Job, style string) string {
	name := nameStart(style) + baseTemplate(job)
	if job.Section != nil && job.Section.Valid() {
		// A clip never takes the full video's name.
		name = strings.TrimSuffix(name, ".%(ext)s") + " " + job.Section.Label() + ".%(ext)s"
	}
	return name
}

// seconds formats a time for --download-sections, which takes plain seconds.
func seconds(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64)
}

// nameStart is what comes before the title for each file-name style. yt-dlp replaces
// path separators inside field values, so only the folder style creates a folder.
func nameStart(style string) string {
	const channel = "%(channel,uploader|Unknown channel).60B"
	switch style {
	case "channel-title":
		return channel + " - "
	case "date-title":
		return "%(upload_date>%Y-%m-%d|undated)s "
	case "channel-folder":
		return channel + "/"
	default:
		return ""
	}
}

func baseTemplate(job domain.Job) string {
	if job.Site != domain.SiteYouTube {
		// The job's video ID is the post's ID (for X, plus "-N" from a post's second video on;
		// letters, digits, and a dash, safe in a template), which yt-dlp's own id isn't.
		// Width x height reads right for the many portrait videos.
		post := "%(title).150B [" + job.VideoID + "]"
		if isVideo(job) || (job.Preset == nil && job.Format == nil) {
			return post + " %(width)sx%(height)s.%(ext)s"
		}
		return post + ".%(ext)s"
	}
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

// stateFilter drops "processing" reports that arrive before the download starts: yt-dlp
// converts thumbnails ahead of the download and reports that as post-processing, and a
// job cannot go back from processing to downloading.
type stateFilter struct{ downloading bool }

func (filter *stateFilter) apply(event Event) Event {
	switch event.State {
	case domain.Downloading:
		filter.downloading = true
	case domain.Processing:
		if !filter.downloading {
			event.State = ""
		}
	}
	return event
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

// siteNames name the sites in failure messages.
var siteNames = map[string]string{domain.SiteX: "X", domain.SiteReddit: "Reddit", domain.SiteInstagram: "Instagram", domain.SiteVimeo: "Vimeo"}

// classifyFor explains a failure in terms of the site it happened on. Other sites' limits
// are their own, so they never pause YouTube downloads ("blocked" is reserved for YouTube).
func classifyFor(site, stderr string) error {
	if site == domain.SiteYouTube {
		return classifyFailure(stderr)
	}
	lower := strings.ToLower(stderr)
	if site == domain.SiteReddit {
		switch {
		case strings.Contains(lower, "no media found"), strings.Contains(lower, "no video formats found"), strings.Contains(lower, "unsupported url"):
			return &Error{Code: "video_unavailable", Message: "This post has no video."}
		case strings.Contains(lower, "login"), strings.Contains(lower, "log in"), strings.Contains(lower, "nsfw"), strings.Contains(lower, "quarantine"), strings.Contains(lower, "http error 403"):
			return &Error{Code: "signin_required", Message: "Reddit shows this post only to signed-in users. Turn on Browser sign-in in Settings, choosing a browser where you're signed in to Reddit, then retry."}
		case strings.Contains(lower, "http error 404"), strings.Contains(lower, "does not exist"), strings.Contains(lower, "removed"), strings.Contains(lower, "deleted"):
			return &Error{Code: "video_unavailable", Message: "This post is unavailable: it may be deleted or removed."}
		}
		return siteFailure(site, stderr)
	}
	if site == domain.SiteVimeo {
		switch {
		case strings.Contains(lower, "embed"), strings.Contains(lower, "referer"), strings.Contains(lower, "domain"):
			return &Error{Code: "video_unavailable", Message: "This video's owner allows it to play only on certain websites, so YTGrab can't download it."}
		case strings.Contains(lower, "logged-in"), strings.Contains(lower, "login"), strings.Contains(lower, "password"), strings.Contains(lower, "private"):
			return &Error{Code: "signin_required", Message: "Vimeo shows this video only to signed-in users or with a password. If you can watch it in your browser, turn on Browser sign-in in Settings with a browser where you're signed in to Vimeo, then retry."}
		case strings.Contains(lower, "http error 404"), strings.Contains(lower, "not found"), strings.Contains(lower, "does not exist"):
			return &Error{Code: "video_unavailable", Message: "This video is unavailable: it may be deleted or private."}
		}
		return siteFailure(site, stderr)
	}
	if site == domain.SiteInstagram {
		switch {
		case strings.Contains(lower, "no video formats found"), strings.Contains(lower, "there is no video in this post"):
			return &Error{Code: "video_unavailable", Message: "This post has no video."}
		case strings.Contains(lower, "rate-limit"), strings.Contains(lower, "rate limit"), strings.Contains(lower, "http error 429"):
			return &Error{Code: "instagram_limited", Message: "Instagram is limiting requests from this network. Wait a few minutes, then retry."}
		case strings.Contains(lower, "empty media response"), strings.Contains(lower, "login"), strings.Contains(lower, "log in"), strings.Contains(lower, "private"):
			return &Error{Code: "signin_required", Message: "Instagram shows this post only to signed-in users (a private account, an age-restricted post, or a deleted one). If you can see it in your browser, turn on Browser sign-in in Settings with a browser where you're signed in to Instagram, then retry."}
		case strings.Contains(lower, "http error 404"), strings.Contains(lower, "not available"):
			return &Error{Code: "video_unavailable", Message: "This post is unavailable: it may be deleted or private."}
		}
		return siteFailure(site, stderr)
	}
	switch {
	case strings.Contains(lower, "no video could be found"), strings.Contains(lower, "no video formats found"):
		return &Error{Code: "video_unavailable", Message: "This post has no video."}
	case strings.Contains(lower, "requires authentication"), strings.Contains(lower, "nsfw"), strings.Contains(lower, "log in"), strings.Contains(lower, "login required"):
		return &Error{Code: "signin_required", Message: "X shows this post only to signed-in users. Turn on Browser sign-in in Settings, choosing a browser where you're signed in to X, then retry."}
	case strings.Contains(lower, "suspended"), strings.Contains(lower, "protected"), strings.Contains(lower, "unavailable"), strings.Contains(lower, "http error 404"), strings.Contains(lower, "does not exist"):
		return &Error{Code: "video_unavailable", Message: "This post is unavailable: it may be deleted, protected, or from a suspended account."}
	}
	return siteFailure(site, stderr)
}

// siteFailure words the general failures for a site other than YouTube.
func siteFailure(site, stderr string) error {
	err := classifyFailure(stderr)
	if toolError, ok := err.(*Error); ok {
		switch toolError.Code {
		case "blocked":
			return &Error{Code: site + "_limited", Message: siteNames[site] + " is limiting requests from this network. Wait a few minutes, then retry."}
		case "video_unavailable":
			return &Error{Code: "video_unavailable", Message: "This post is unavailable or private."}
		case "forbidden", "youtube_changed":
			return &Error{Code: toolError.Code, Message: siteNames[site] + " refused the download. Update yt-dlp from the tools panel and retry later; if it keeps happening, turn on Browser sign-in."}
		case "download_failed":
			return &Error{Code: "download_failed", Message: "yt-dlp could not download this post's video. Check that yt-dlp is up to date and retry."}
		}
	}
	return err
}

func classifyFailure(stderr string) error {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "drm protected"):
		return &Error{Code: "drm_protected", Message: "This video is copy-protected (DRM), so it can't be downloaded."}
	// YouTube's own reasons, checked before the general "not available" below.
	case strings.Contains(lower, "confirm your age"), strings.Contains(lower, "age-restricted"), strings.Contains(lower, "inappropriate for some users"):
		return &Error{Code: "signin_required", Message: "YouTube shows this video only to signed-in adults. Turn on Browser sign-in in Settings with a browser where you're signed in to YouTube, then retry."}
	case strings.Contains(lower, "members-only"), strings.Contains(lower, "join this channel"):
		return &Error{Code: "members_only", Message: "This video is for the channel's paying members only. YTGrab can download it only if you're a member: turn on Browser sign-in with a browser signed in to that account."}
	case strings.Contains(lower, "premieres in"), strings.Contains(lower, "live event will begin"), strings.Contains(lower, "this live event"):
		return &Error{Code: "not_started", Message: "This premiere or live stream hasn't started yet. Retry after it has aired."}
	case strings.Contains(lower, "requested format is not available"):
		return &Error{Code: "format_unavailable", Message: "That quality isn't offered for this video any more. Check its formats again, or use a preset like Best quality."}
	case strings.Contains(lower, "challenge solving failed"), strings.Contains(lower, "signature solving failed"), strings.Contains(lower, "no supported javascript runtime"):
		return &Error{Code: "youtube_changed", Message: "YouTube changed how its videos are protected, and this yt-dlp can't keep up yet. Update yt-dlp from the tools panel (YTGrab also does it by itself), then retry."}
	case strings.Contains(lower, "no space left on device"), strings.Contains(lower, "not enough space on the disk"):
		return &Error{Code: "disk_full", Message: "The drive with your download folder is full. Free up space or choose another folder, then Retry."}
	// Checked first and by specific phrases: YouTube's bot check also mentions cookies.
	case strings.Contains(lower, "cookie database"), strings.Contains(lower, "cookies database"), strings.Contains(lower, "failed to decrypt"), strings.Contains(lower, "failed to load cookies"), strings.Contains(lower, "unsupported browser"):
		return &Error{Code: "cookies_failed", Message: "YTGrab couldn't use your browser's sign-in. Close that browser and retry, or choose another browser in the settings (on Windows, Firefox works best)."}
	case strings.Contains(lower, "http error 429"), strings.Contains(lower, "sign in to confirm you're not a bot"), strings.Contains(lower, "sign in to confirm you’re not a bot"), strings.Contains(lower, "too many requests"):
		return &Error{Code: "blocked", Message: "YouTube is limiting requests from this network. Wait a while, then retry."}
	case strings.Contains(lower, "private video"), strings.Contains(lower, "video unavailable"), strings.Contains(lower, "not available"), strings.Contains(lower, "does not exist"):
		return &Error{Code: "video_unavailable", Message: "This video is unavailable or private."}
	case strings.Contains(lower, "ffmpeg not found"), strings.Contains(lower, "ffprobe not found"):
		return &Error{Code: "dependency_missing", Message: "Install ffmpeg and ffprobe, then retry."}
	case strings.Contains(lower, "http error 403"):
		return &Error{Code: "forbidden", Message: "YouTube refused the download (HTTP 403). This usually clears once yt-dlp catches up with a YouTube change: update yt-dlp from the tools panel and retry later. If it keeps happening, turn on Browser sign-in."}
	case strings.Contains(lower, "timed out"), strings.Contains(lower, "connection"), strings.Contains(lower, "http error 5"):
		return &Error{Code: "network", Message: "The download failed because of a network error. Retry shortly."}
	default:
		return &Error{Code: "download_failed", Message: "yt-dlp could not download this video. Check that yt-dlp is up to date and retry."}
	}
}

// errorDetail keeps yt-dlp's last ERROR and WARNING lines (or its last lines when there
// are none), up to 1500 characters, for the diagnostic report.
func errorDetail(stderr string) string {
	var lines, kept []string
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r", ""), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "ERROR:") || strings.HasPrefix(line, "WARNING:") {
			kept = append(kept, line)
		}
	}
	if len(kept) == 0 {
		kept = lines
	}
	if len(kept) > 6 {
		kept = kept[len(kept)-6:]
	}
	detail := strings.Join(kept, "\n")
	if len(detail) > 1500 {
		detail = strings.ToValidUTF8(detail[len(detail)-1500:], "")
	}
	return detail
}

// removeThumbnails deletes the cover images yt-dlp saved for a download that failed for
// good, before it could embed them: files named with "[videoID]" and an image extension.
// Partial downloads are kept, so a retry still resumes.
func removeThumbnails(dir, videoID string) {
	if !plainID.MatchString(videoID) {
		return // nothing that a glob pattern would read differently
	}
	for _, ext := range []string{"jpg", "webp", "png"} {
		// "[[]" matches a literal "[": a bare one would start a character class.
		matches, _ := filepath.Glob(filepath.Join(dir, "*[[]"+videoID+"]."+ext))
		for _, match := range matches {
			_ = os.Remove(match)
		}
	}
}

// plainID matches the video IDs every site uses: letters, digits, "_", "-", and ".".
var plainID = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
