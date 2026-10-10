package ytdlp

import (
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestWithLanguageTriesTheTrackFirst(t *testing.T) {
	for selector, want := range map[string]string{
		"bv*[height<=720]+ba/b[height<=720]":          "bv*[height<=720]+ba[language=es-US]/bv*[height<=720]+ba/b[height<=720]",
		"ba[ext=m4a]":                                 "ba[language=es-US][ext=m4a]/ba[ext=m4a]",
		"ba[acodec=opus][abr<=60]/wa[acodec=opus]/wa": "ba[language=es-US][acodec=opus][abr<=60]/wa[language=es-US][acodec=opus]/wa[language=es-US]/ba[acodec=opus][abr<=60]/wa[acodec=opus]/wa",
		"398+ba[ext=m4a]/398+ba/398":                  "398+ba[language=es-US][ext=m4a]/398+ba[language=es-US]/398+ba[ext=m4a]/398+ba/398",
		"251-5":                                       "251-5", // an exact track already names its language
	} {
		if got := withLanguage(selector, "es-US"); got != want {
			t.Errorf("withLanguage(%q) =\n  %q\nwant\n  %q", selector, got, want)
		}
	}
	if got := withLanguage("bv*+ba/b", ""); got != "bv*+ba/b" {
		t.Errorf("no language = %q", got)
	}
	if got := withLanguage("bv*+ba/b", "es]+bv"); got != "bv*+ba/b" {
		t.Errorf("an invalid language reached yt-dlp: %q", got)
	}
}

func TestAudioLanguageAndPlaysEverywhereReachYtdlp(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/Txzj3pNt20o", domain.Video720)
	job.AudioLanguage = "es-US"
	args := buildArgs(job, config.Config{})
	if got := argAfter(args, "-f"); !strings.HasPrefix(got, "bv*[height<=720]+ba[language=es-US]/") {
		t.Errorf("720p in Spanish = %q", got)
	}
	if got := argAfter(args, "-o"); !strings.HasSuffix(got, " %(height)sp es-US.%(ext)s") {
		t.Errorf("name = %q", got)
	}
	compat, _ := domain.NewJob("https://youtu.be/Txzj3pNt20o", domain.VideoCompat)
	args = buildArgs(compat, config.Config{})
	if got := argAfter(args, "-f"); !strings.HasPrefix(got, "bv*[vcodec^=avc1][height<=1080]+ba[acodec^=mp4a]/") {
		t.Errorf("plays everywhere = %q", got)
	}
	if got := argAfter(args, "--merge-output-format"); got != "mp4" {
		t.Errorf("container = %q", got)
	}
	if !isVideo(compat) || !strings.Contains(argAfter(args, "-o"), "%(height)sp") {
		t.Error("plays everywhere isn't treated as video")
	}
}
