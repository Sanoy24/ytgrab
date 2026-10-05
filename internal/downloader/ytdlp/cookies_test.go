package ytdlp

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

// When YTGRAB_FAKE_YTDLP_LOG is set, this test binary stands in for yt-dlp: it records its
// arguments, fails like yt-dlp does when a browser's cookies can't be decrypted, and
// otherwise "downloads" a file into the -P folder.
func TestMain(m *testing.M) {
	if log := os.Getenv("YTGRAB_FAKE_YTDLP_LOG"); log != "" {
		args := os.Args[1:]
		if f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_, _ = f.WriteString(strings.Join(args, " ") + "\n")
			_ = f.Close()
		}
		if slices.Contains(args, "--cookies-from-browser") {
			os.Stderr.WriteString("ERROR: Failed to decrypt with DPAPI. See  https://github.com/yt-dlp/yt-dlp/issues/10927  for more info\nERROR: failed to load cookies\n")
			os.Exit(1)
		}
		dir := args[slices.Index(args, "-P")+1]
		file := filepath.Join(dir, "Clip [jNQXAC9IVRw].m4a")
		_ = os.WriteFile(file, []byte("audio"), 0o600)
		quoted, _ := json.Marshal(file)
		os.Stdout.WriteString(pathPrefix + string(quoted) + "\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeYtdlp(t *testing.T) (cfg config.Config, log string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	tools := t.TempDir()
	name := "yt-dlp"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	src, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.OpenFile(filepath.Join(tools, name), os.O_CREATE|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	dst.Close()
	log = filepath.Join(t.TempDir(), "runs.log")
	t.Setenv("YTGRAB_FAKE_YTDLP_LOG", log)
	t.Setenv("PATH", "") // no real runtimes or FFmpeg
	return config.Config{ToolsDir: tools, DownloadsDir: t.TempDir()}, log
}

// A browser whose sign-in can't be read shouldn't stop a download that may not need it.
func TestUnreadableBrowserSignInRetriesWithout(t *testing.T) {
	cfg, log := fakeYtdlp(t)
	job, err := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	if err != nil {
		t.Fatal(err)
	}
	downloader := Downloader{Config: cfg, CookiesBrowser: func() string { return "chrome" }}
	result, err := downloader.Download(context.Background(), job, func(Event) error { return nil })
	if err != nil || filepath.Base(result.OutputPath) != "Clip [jNQXAC9IVRw].m4a" {
		t.Fatalf("Download = %+v, %v", result, err)
	}
	data, _ := os.ReadFile(log)
	runs := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(runs) != 2 || !strings.Contains(runs[0], "--cookies-from-browser chrome") || strings.Contains(runs[1], "--cookies") {
		t.Errorf("runs = %q", runs)
	}
}

func TestCookieFileIsPassedByPath(t *testing.T) {
	cfg, log := fakeYtdlp(t)
	cookies := filepath.Join(t.TempDir(), "cookies.txt")
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	downloader := Downloader{Config: cfg, CookiesBrowser: func() string { return cookies }}
	if _, err := downloader.Download(context.Background(), job, func(Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "--cookies "+cookies) || strings.Count(strings.TrimSpace(string(data)), "\n") != 0 {
		t.Errorf("runs = %q", data)
	}
}
