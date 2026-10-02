package app

import (
	"context"
	"io"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/api"
)

func TestAutoUpdateStep(t *testing.T) {
	for _, c := range []struct {
		name            string
		on              bool
		running         int
		current, latest string
		want            string
		updates         int
	}{
		{"turned off", false, 0, "2026.08.19", "2026.09.30", "off", 0},
		{"downloading", true, 1, "2026.08.19", "2026.09.30", "busy", 0},
		{"already current", true, 0, "2026.09.30", "2026.09.30", "current", 0},
		{"release unknown", true, 0, "2026.08.19", "", "current", 0},
		{"not installed", true, 0, "", "2026.09.30", "current", 0},
		{"newer release", true, 0, "2026.08.19", "2026.09.30", "updated", 1},
		{"hotfix release", true, 0, "2026.09.30", "2026.09.30.1", "updated", 1},
	} {
		updates := 0
		auto := ytdlpAutoUpdate{
			enabled:  func() bool { return c.on },
			running:  func() int { return c.running },
			versions: func(context.Context) (string, string) { return c.current, c.latest },
			update: func(context.Context) (api.YtdlpUpdate, error) {
				updates++
				return api.YtdlpUpdate{Version: c.latest, Previous: c.current, Updated: true}, nil
			},
			output: io.Discard,
		}
		if got := auto.step(context.Background()); got != c.want || updates != c.updates {
			t.Errorf("%s: step = %q with %d updates, want %q with %d", c.name, got, updates, c.want, c.updates)
		}
	}
}
