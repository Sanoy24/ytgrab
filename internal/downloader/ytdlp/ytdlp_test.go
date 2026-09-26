package ytdlp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ytgrab/internal/config"
	"ytgrab/internal/domain"
)

func TestMachineOutputParsing(t *testing.T) {
	event, ok := parseEvent(`YTGRAB_PROGRESS:{"downloaded_bytes":1024,"total_bytes":null,"total_bytes_estimate":2048,"speed":512.5,"eta":2}`)
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
