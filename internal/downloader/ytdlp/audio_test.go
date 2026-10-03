package ytdlp

import (
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestAudioPresets(t *testing.T) {
	for preset, want := range map[domain.Preset]struct {
		selector, format string
		cover            bool
	}{
		domain.AudioOpus: {"ba[acodec=opus]/ba", "opus", true},
		domain.AudioFLAC: {"ba", "flac", true},
		domain.AudioWAV:  {"ba", "wav", false},
	} {
		job, err := domain.NewJob("https://youtu.be/jNQXAC9IVRw", preset)
		if err != nil {
			t.Fatalf("%s: %v", preset, err)
		}
		args := buildArgs(job, config.Config{SubtitlesMode: "embed", SubtitlesLang: "en"})
		if argAfter(args, "-f") != want.selector || argAfter(args, "--audio-format") != want.format {
			t.Errorf("%s: args = %v", preset, args)
		}
		cover := false
		for _, a := range args {
			if a == "--embed-subs" {
				t.Errorf("%s: subtitles requested for audio", preset)
			}
			cover = cover || a == "--embed-thumbnail"
		}
		if cover != want.cover {
			t.Errorf("%s: cover art = %v, want %v", preset, cover, want.cover)
		}
	}
}
