package ytdlp

import (
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestSectionDownloadsOnlyThatPart(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	if joined := strings.Join(buildArgs(job, config.Config{}), " "); strings.Contains(joined, "--download-sections") {
		t.Fatalf("whole video expected: %s", joined)
	}
	job.Section = &domain.Section{Start: 2.5, End: 12}
	args := buildArgs(job, config.Config{})
	if got := argAfter(args, "--download-sections"); got != "*2.5-12" {
		t.Fatalf("--download-sections = %q", got)
	}
	if got := argAfter(args, "-o"); got != "%(title).150B [%(id)s] %(height)sp 3s-12s.%(ext)s" {
		t.Fatalf("output template = %q", got)
	}
	if args[len(args)-1] != job.URL || args[len(args)-2] != "--" {
		t.Fatal("URL must stay the final argument")
	}
}
