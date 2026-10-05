package ytdlp

import (
	"strings"
	"testing"
)

// Real yt-dlp and YouTube wordings, so a change in either shows up here.
func TestYouTubeFailuresInPlainLanguage(t *testing.T) {
	for stderr, code := range map[string]string{
		"ERROR: [youtube] abc: Sign in to confirm your age. This video may be inappropriate for some users.": "signin_required",
		"WARNING: [youtube] abc: This video is age-restricted; some formats may be missing":                  "signin_required",
		"ERROR: [youtube] abc: Join this channel to get access to members-only content like this video":     "members_only",
		"ERROR: [youtube] abc: Premieres in 10 hours":                                                         "not_started",
		"ERROR: [youtube] abc: This live event will begin in a few moments.":                                 "not_started",
		"ERROR: [youtube] abc: Requested format is not available. Use --list-formats for a list":             "format_unavailable",
		"WARNING: [youtube] abc: n challenge solving failed: Some formats may be missing.":                   "youtube_changed",
		"ERROR: unable to download video data: HTTP Error 403: Forbidden":                                    "forbidden",
		"ERROR: [youtube] abc: Video unavailable":                                                             "video_unavailable",
		"ERROR: [youtube] abc: Sign in to confirm you're not a bot":                                          "blocked",
	} {
		if err, ok := classifyFailure(stderr).(*Error); !ok || err.Code != code {
			t.Errorf("%q -> %v", stderr, err)
		}
	}
}

func TestErrorDetailKeepsYtdlpsOwnLines(t *testing.T) {
	stderr := "[youtube] Extracting URL\r\n[info] abc: Downloading 1 format\nWARNING: [youtube] abc: n challenge solving failed\nERROR: unable to download video data: HTTP Error 403: Forbidden\n"
	if got := errorDetail(stderr); got != "WARNING: [youtube] abc: n challenge solving failed\nERROR: unable to download video data: HTTP Error 403: Forbidden" {
		t.Errorf("detail = %q", got)
	}
	if got := errorDetail("something odd\nhappened"); got != "something odd\nhappened" {
		t.Errorf("no ERROR lines = %q", got)
	}
	if got := errorDetail(strings.Repeat("ERROR: x\n", 1000)); len(got) > 1500 {
		t.Errorf("detail is %d characters", len(got))
	}
}

func TestOtherSitesDontBlameYouTube(t *testing.T) {
	err, ok := classifyFor("reddit", "ERROR: [Reddit] abc: HTTP Error 403: Forbidden").(*Error)
	if !ok || err.Code == "" || strings.Contains(err.Message, "YouTube") || !strings.Contains(err.Message, "Reddit") {
		t.Errorf("reddit 403 = %+v", err)
	}
}
