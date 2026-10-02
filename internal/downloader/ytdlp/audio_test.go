package ytdlp

import (
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestAudioPresets(t *testing.T) {
	for preset, want := range map[domain.Preset]struct{ selector, format string }{
		domain.AudioOpus: {"ba[acodec=opus]/ba", "opus"},
		domain.AudioFLAC: {"ba", "flac"},
		domain.AudioWAV:  {"ba", "wav"},
	} {
		job, err := domain.NewJob("https://youtu.be/jNQXAC9IVRw", preset)
		if err != nil {
			t.Fatalf("%s: %v", preset, err)
		}
		args := buildArgs(job, config.Config{SubtitlesMode: "embed", SubtitlesLang: "en"})
		if argAfter(args, "-f") != want.selector || argAfter(args, "--audio-format") != want.format {
			t.Errorf("%s: args = %v", preset, args)
		}
		for _, a := range args {
			if a == "--embed-subs" || a == "--embed-thumbnail" {
				t.Errorf("%s: unexpected %s", preset, a)
			}
		}
	}
}
