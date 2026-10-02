package ytdlp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/diskspace"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestMachineOutputParsing(t *testing.T) {
	event, ok := parseEvent(`YTGRAB_PROGRESS:{"progress":{"downloaded_bytes":1024,"total_bytes":null,"total_bytes_estimate":2048,"speed":512.5,"eta":2},"vcodec":"avc1"}`)
	if !ok || event.Progress == nil || event.Progress.DownloadedBytes != 1024 || *event.Progress.TotalBytes != 2048 || *event.Progress.ETASeconds != 2 {
		t.Fatalf("progress event = %+v, %v", event, ok)
	}
	event, ok = parseEvent(`YTGRAB_TITLE:"Example: \"quoted\""`)
	if !ok || event.Title != `Example: "quoted"` {
		t.Fatalf("title event = %+v, %v", event, ok)
	}
	event, ok = parseEvent(`YTGRAB_PATH:"C:\\Downloads\\video.mp4"`)
	if !ok || !strings.Contains(event.OutputPath, "video.mp4") {
		t.Fatalf("path event = %+v, %v", event, ok)
	}
}

func TestArgumentsKeepURLAsOneValue(t *testing.T) {
	job, err := domain.NewJob("https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PL123", domain.AudioMP3)
	if err != nil {
		t.Fatal(err)
	}
	args := buildArgs(job, config.Config{DownloadsDir: t.TempDir()})
	if args[len(args)-1] != job.URL || args[len(args)-2] != "--" {
		t.Fatalf("URL was not the final argument: %v", args)
	}
	joined := strings.Join(args, " ")
	for _, required := range []string{"--no-playlist", "--progress-template", "--audio-format mp3"} {
		if !strings.Contains(joined, required) {
			t.Errorf("missing %q from arguments", required)
		}
	}
}

func TestConfirmOutputStaysInDownloadDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := confirmOutput(dir, path); err != nil || got != path {
		t.Fatalf("confirmOutput = %q, %v", got, err)
	}
	if _, err := confirmOutput(dir, filepath.Join(dir, "..", "other.mp4")); err == nil {
		t.Fatal("output outside download directory was accepted")
	}
}

func TestClassifyBotCheck(t *testing.T) {
	for _, message := range []string{"HTTP Error 429: Too Many Requests", "Sign in to confirm you're not a bot"} {
		err, ok := classifyFailure(message).(*Error)
		if !ok || err.Code != "blocked" {
			t.Fatalf("classifyFailure(%q) = %v", message, err)
		}
	}
}

func TestProgressReportsStreamKind(t *testing.T) {
	for _, test := range []struct {
		vcodec string
		want   string
	}{
		{`"avc1.4d401e"`, "video"},
		{`"none"`, "audio"},
		{`null`, ""},
	} {
		line := `YTGRAB_PROGRESS:{"progress":{"downloaded_bytes":10,"total_bytes":100,"speed":5,"eta":18},"vcodec":` + test.vcodec + `}`
		event, ok := parseEvent(line)
		if !ok || event.Progress == nil || event.Progress.DownloadedBytes != 10 || event.Progress.Stream != test.want {
			t.Fatalf("parseEvent(%s) = %+v, %v", test.vcodec, event.Progress, ok)
		}
	}
}

// Different qualities of one video must not share a file name, or yt-dlp reports the
// earlier file as "already downloaded" and the job completes with the wrong quality.
func TestOutputNameIncludesQuality(t *testing.T) {
	video, _ := domain.NewFormatJob("https://youtu.be/jNQXAC9IVRw", domain.FormatSelection{Kind: "video", ID: "137", Ext: "mp4", Label: "Video · 1080p"})
	audio, _ := domain.NewFormatJob("https://youtu.be/jNQXAC9IVRw", domain.FormatSelection{Kind: "audio", ID: "140", Ext: "m4a", Label: "Audio · M4A"})
	preset720, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	mp3, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioMP3)
	for _, test := range []struct {
		job  domain.Job
		want string
	}{
		{video, "%(title).150B [%(id)s] %(height)sp.%(ext)s"},
		{preset720, "%(title).150B [%(id)s] %(height)sp.%(ext)s"},
		{audio, "%(title).150B [%(id)s] %(abr).0fk.%(ext)s"},
		{mp3, "%(title).150B [%(id)s].%(ext)s"},
	} {
		args := buildArgs(test.job, config.Config{})
		if got := argAfter(args, "-o"); got != test.want {
			t.Errorf("output template = %q, want %q", got, test.want)
		}
	}
}

func argAfter(args []string, flag string) string {
	for i := range args[:len(args)-1] {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

func TestBrowserSignInIsPassedOnlyWhenSet(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	if joined := strings.Join(buildArgs(job, config.Config{}), " "); strings.Contains(joined, "--cookies") {
		t.Fatalf("cookies used while sign-in is off: %s", joined)
	}
	args := buildArgs(job, config.Config{CookiesBrowser: "firefox"})
	if got := argAfter(args, "--cookies-from-browser"); got != "firefox" {
		t.Fatalf("--cookies-from-browser = %q in %v", got, args)
	}
	if args[len(args)-1] != job.URL || args[len(args)-2] != "--" {
		t.Fatal("URL must stay the final argument")
	}
	// Defense in depth: anything that isn't a plain browser name is ignored.
	if joined := strings.Join(buildArgs(job, config.Config{CookiesBrowser: "firefox --exec x"}), " "); strings.Contains(joined, "--cookies") {
		t.Fatalf("unsafe browser value passed through: %s", joined)
	}
}

func TestCookieFailuresAreExplained(t *testing.T) {
	for _, stderr := range []string{
		"ERROR: Could not copy Chrome cookie database. See https://github.com/yt-dlp/yt-dlp/issues/7271 for more info",
		"ERROR: Failed to decrypt with DPAPI. See https://github.com/yt-dlp/yt-dlp/issues/10927 for more info",
		"ERROR: could not find firefox cookies database in /home/me/.mozilla/firefox",
	} {
		err, ok := classifyFailure(stderr).(*Error)
		if !ok || err.Code != "cookies_failed" {
			t.Errorf("classifyFailure(%q) = %v", stderr, err)
		}
	}
	// The bot check mentions --cookies-from-browser but is still a block.
	err, _ := classifyFailure("ERROR: [youtube] x: Sign in to confirm you're not a bot. Use --cookies-from-browser or --cookies for the authentication.").(*Error)
	if err == nil || err.Code != "blocked" {
		t.Fatalf("bot check classified as %v", err)
	}
}

func TestMetadataIsEmbedded(t *testing.T) {
	m4a, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	mp3, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioMP3)
	video, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.VideoBest)
	opus, _ := domain.NewFormatJob("https://youtu.be/jNQXAC9IVRw", domain.FormatSelection{Kind: "audio", ID: "251", Ext: "webm", Label: "Audio · WEBM"})
	pickedM4A, _ := domain.NewFormatJob("https://youtu.be/jNQXAC9IVRw", domain.FormatSelection{Kind: "audio", ID: "140", Ext: "m4a", Label: "Audio · M4A"})
	for name, test := range map[string]struct {
		job       domain.Job
		thumbnail bool
	}{
		"m4a preset":   {m4a, true},
		"mp3 preset":   {mp3, true},
		"picked m4a":   {pickedM4A, true},
		"picked opus":  {opus, false}, // WebM can't hold cover art; embedding would fail the job
		"video preset": {video, false},
	} {
		joined := strings.Join(buildArgs(test.job, config.Config{}), " ")
		if !strings.Contains(joined, "--embed-metadata") || !strings.Contains(joined, "--embed-chapters") {
			t.Errorf("%s: metadata not embedded: %s", name, joined)
		}
		if got := strings.Contains(joined, "--embed-thumbnail"); got != test.thumbnail {
			t.Errorf("%s: --embed-thumbnail = %v, want %v", name, got, test.thumbnail)
		}
	}
}

// yt-dlp converts thumbnails before downloading and reports it as post-processing; that
// must not move the job to "processing" before the download has started.
func TestProcessingBeforeTheDownloadIsIgnored(t *testing.T) {
	var filter stateFilter
	before := filter.apply(Event{State: domain.Processing})
	if before.State != "" {
		t.Fatalf("processing before download = %q, want ignored", before.State)
	}
	if got := filter.apply(Event{State: domain.Downloading, Progress: &domain.Progress{DownloadedBytes: 1}}); got.State != domain.Downloading {
		t.Fatalf("download event = %q", got.State)
	}
	if got := filter.apply(Event{State: domain.Processing}); got.State != domain.Processing {
		t.Fatalf("processing after download = %q", got.State)
	}
	if got := filter.apply(Event{Title: "x"}); got.Title != "x" {
		t.Fatal("other fields must pass through")
	}
}

func TestDownloadRefusedWhenTheDriveIsNearlyFull(t *testing.T) {
	freeSpace = func(string) (uint64, error) { return 120_000_000, nil }
	defer func() { freeSpace = diskspace.Free }()
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	_, err := Downloader{Config: config.Config{DownloadsDir: t.TempDir()}}.Download(context.Background(), job, nil)
	var toolError *Error
	if !errors.As(err, &toolError) || toolError.Code != "disk_full" || !strings.Contains(toolError.Message, "120 MB") {
		t.Fatalf("Download on a full drive = %v", err)
	}
}

func TestOutOfSpaceDuringDownload(t *testing.T) {
	for _, stderr := range []string{
		"ERROR: unable to write data: [Errno 28] No space left on device",
		"ERROR: unable to write data: [WinError 112] There is not enough space on the disk",
	} {
		if err, ok := classifyFailure(stderr).(*Error); !ok || err.Code != "disk_full" {
			t.Errorf("classifyFailure(%q) = %v", stderr, err)
		}
	}
}

func TestSubtitlesOnlyForVideoWhenTurnedOn(t *testing.T) {
	video, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	picked, _ := domain.NewFormatJob("https://youtu.be/jNQXAC9IVRw", domain.FormatSelection{Kind: "video", ID: "137", Ext: "mp4", Label: "1080p"})
	audio, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioMP3)

	if joined := strings.Join(buildArgs(video, config.Config{SubtitlesMode: "off", SubtitlesLang: "en"}), " "); strings.Contains(joined, "subs") {
		t.Fatalf("subtitles requested while off: %s", joined)
	}
	embed := buildArgs(picked, config.Config{SubtitlesMode: "embed", SubtitlesLang: "am"})
	if argAfter(embed, "--sub-langs") != "am" || !slices.Contains(embed, "--embed-subs") || !slices.Contains(embed, "--write-auto-subs") || argAfter(embed, "--compat-options") != "no-keep-subs" {
		t.Fatalf("embed args = %v", embed)
	}
	file := buildArgs(video, config.Config{SubtitlesMode: "file", SubtitlesLang: "en"})
	if argAfter(file, "--convert-subs") != "srt" || slices.Contains(file, "--embed-subs") {
		t.Fatalf("file args = %v", file)
	}
	if joined := strings.Join(buildArgs(audio, config.Config{SubtitlesMode: "embed", SubtitlesLang: "en"}), " "); strings.Contains(joined, "subs") {
		t.Fatalf("subtitles requested for audio: %s", joined)
	}
	// Defense in depth: anything but a short language code is ignored.
	if joined := strings.Join(buildArgs(video, config.Config{SubtitlesMode: "embed", SubtitlesLang: "en --exec x"}), " "); strings.Contains(joined, "subs") {
		t.Fatalf("unsafe language passed through: %s", joined)
	}
	if file[len(file)-1] != video.URL || file[len(file)-2] != "--" {
		t.Fatal("URL must stay the final argument")
	}
}

func TestSpeedLimitIsPassedWhenSet(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.VideoBest)
	if joined := strings.Join(buildArgs(job, config.Config{}), " "); strings.Contains(joined, "--limit-rate") {
		t.Fatalf("limited without a limit: %s", joined)
	}
	if got := argAfter(buildArgs(job, config.Config{SpeedLimitKBps: 2000}), "--limit-rate"); got != "2000K" {
		t.Fatalf("--limit-rate = %q", got)
	}
}
