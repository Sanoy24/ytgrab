package ytdlp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
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
