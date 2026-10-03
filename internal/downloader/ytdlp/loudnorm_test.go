package ytdlp

import (
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestLoudnessOnlyForConvertedAudio(t *testing.T) {
	for preset, want := range map[domain.Preset]bool{
		domain.AudioMP3: true, domain.AudioFLAC: true, domain.AudioWAV: true,
		domain.AudioM4A: false, domain.AudioOpus: false, domain.VideoBest: false,
	} {
		job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", preset)
		on := strings.Contains(strings.Join(buildArgs(job, config.Config{NormalizeAudio: true}), " "), "loudnorm")
		off := strings.Contains(strings.Join(buildArgs(job, config.Config{}), " "), "loudnorm")
		if on != want || off {
			t.Errorf("%s: normalized when on = %v (want %v), when off = %v", preset, on, want, off)
		}
	}
}
