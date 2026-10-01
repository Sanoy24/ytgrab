package deps

import (
	"strings"
	"testing"
	"time"
)

func TestYtdlpAge(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for version, want := range map[string]int{
		"2026.08.19":   43,
		"2026.09.30":   1,
		"2026.09.30.1": 1, // hotfix releases add a fourth part
		"nightly":      -1,
		"":             -1,
	} {
		days, ok := ytdlpAgeDays(version, now)
		if want < 0 {
			if ok {
				t.Errorf("ytdlpAgeDays(%q) = %d, want unknown", version, days)
			}
			continue
		}
		if !ok || days != want {
			t.Errorf("ytdlpAgeDays(%q) = %d, %v; want %d", version, days, ok, want)
		}
	}
}

func reportWith(version string) Report {
	return Report{Dependencies: []Tool{{Name: "yt-dlp", Available: true, Version: version, Message: "Available."}, {Name: "ffmpeg", Available: true, Version: "8.0.1"}}}
}

func TestOutdatedMeansANewerReleaseExists(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	// The newest release is itself 43 days old: nothing to update.
	current := reportWith("2026.08.19")
	MarkOutdated(&current, "2026.08.19", now)
	if current.Dependencies[0].Outdated {
		t.Fatalf("latest version flagged: %+v", current.Dependencies[0])
	}

	behind := reportWith("2026.08.19")
	MarkOutdated(&behind, "2026.09.28", now)
	if tool := behind.Dependencies[0]; !tool.Outdated || !strings.Contains(tool.Message, "2026.09.28") {
		t.Fatalf("older version not flagged: %+v", tool)
	}

	hotfix := reportWith("2026.09.28")
	MarkOutdated(&hotfix, "2026.09.28.1", now)
	if !hotfix.Dependencies[0].Outdated {
		t.Fatal("hotfix release not detected")
	}

	// Offline: only very old versions are flagged.
	offline := reportWith("2026.08.19")
	MarkOutdated(&offline, "", now)
	if offline.Dependencies[0].Outdated {
		t.Fatal("43-day-old version flagged without knowing the latest release")
	}
	ancient := reportWith("2026.05.01")
	MarkOutdated(&ancient, "", now)
	if tool := ancient.Dependencies[0]; !tool.Outdated || !strings.Contains(tool.Message, "days old") {
		t.Fatalf("very old version not flagged offline: %+v", tool)
	}
	if ancient.Dependencies[1].Outdated {
		t.Fatal("only yt-dlp is checked")
	}
}
