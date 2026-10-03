package ytdlp

import (
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestFileNameStyles(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	for style, want := range map[string]string{
		"":               "%(title).150B [%(id)s] %(height)sp.%(ext)s",
		"title":          "%(title).150B [%(id)s] %(height)sp.%(ext)s",
		"channel-title":  "%(channel,uploader|Unknown channel).60B - %(title).150B [%(id)s] %(height)sp.%(ext)s",
		"date-title":     "%(upload_date>%Y-%m-%d|undated)s %(title).150B [%(id)s] %(height)sp.%(ext)s",
		"channel-folder": "%(channel,uploader|Unknown channel).60B/%(title).150B [%(id)s] %(height)sp.%(ext)s",
		"../unknown":     "%(title).150B [%(id)s] %(height)sp.%(ext)s",
	} {
		if got := argAfter(buildArgs(job, config.Config{FileNames: style}), "-o"); got != want {
			t.Errorf("%q: -o %q, want %q", style, got, want)
		}
	}
}
