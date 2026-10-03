package ytdlp

import (
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestSponsorBlock(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	for mode, want := range map[string]string{"": "", "off": "", "mark": "--sponsorblock-mark", "remove": "--sponsorblock-remove", "--exec x": ""} {
		args := buildArgs(job, config.Config{SponsorBlock: mode})
		joined := strings.Join(args, " ")
		if want == "" {
			if strings.Contains(joined, "sponsorblock") {
				t.Errorf("%q: unexpected SponsorBlock in %s", mode, joined)
			}
			continue
		}
		if argAfter(args, want) != "sponsor,selfpromo,interaction" {
			t.Errorf("%q: args = %v", mode, args)
		}
		if args[len(args)-1] != job.URL || args[len(args)-2] != "--" {
			t.Fatal("URL must stay the final argument")
		}
	}
}
