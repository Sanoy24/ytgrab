package ytdlp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestInstagramArguments(t *testing.T) {
	job, err := domain.NewJob("https://www.instagram.com/reel/DeAVy0uTbPn/?utm_source=ig_web_copy_link", domain.Video1080)
	if err != nil {
		t.Fatal(err)
	}
	args := buildArgs(job, config.Config{SponsorBlock: "remove", SubtitlesMode: "embed"})
	joined := strings.Join(args, " ")
	if !strings.HasSuffix(joined, "--playlist-items 1 -f bv*[height<=1080]+ba/b[height<=1080] --merge-output-format mp4/mkv --merge-output-format mp4 -- https://www.instagram.com/p/DeAVy0uTbPn/") {
		t.Errorf("args = %s", joined)
	}
	if strings.Contains(joined, "sponsorblock") || strings.Contains(joined, "--write-subs") {
		t.Errorf("YouTube-only options used for Instagram: %s", joined)
	}
	if got := argAfter(args, "-o"); got != "%(title).150B [DeAVy0uTbPn] %(width)sx%(height)s.%(ext)s" {
		t.Errorf("name = %q", got)
	}
	second, _ := domain.NewJob(domain.InstagramItemURL("BQ0eAlwhDrw", 2), domain.AudioM4A)
	args = buildArgs(second, config.Config{})
	if joined := strings.Join(args, " "); !strings.Contains(joined, "--playlist-items 2") || !strings.HasSuffix(joined, "-- https://www.instagram.com/p/BQ0eAlwhDrw/") {
		t.Errorf("second item args = %s", joined)
	}
	if got := argAfter(args, "-o"); got != "%(title).150B [BQ0eAlwhDrw.2].%(ext)s" {
		t.Errorf("second item name = %q", got)
	}
}

func TestInstagramInspection(t *testing.T) {
	reel := func(id, thumb string) string {
		return fmt.Sprintf(`{"id":%q,"display_id":%q,"title":"Video by apeexmind","thumbnail":"https://instagram.fadd1-1.fna.fbcdn.net/v/%s.jpg?stp=x",
			"formats":[
				{"format_id":"dash-1438230828410957a","ext":"m4a","vcodec":"none","acodec":"mp4a.40.5","protocol":"https","tbr":57.172,"abr":57.172},
				{"format_id":"1","ext":"mp4","protocol":"https"},
				{"format_id":"dash-1438244541742919v","ext":"mp4","vcodec":"vp09.00.40.08","acodec":"none","width":1080,"height":1920,"protocol":"https","tbr":1053.066}]}`, id, id, thumb)
	}
	single, err := parseInspection([]byte(reel("DeAVy0uTbPn", "a")), "DeAVy0uTbPn", domain.SiteInstagram)
	if err != nil || single.Site != "instagram" || single.Thumbnail == "" || len(single.Video) != 1 || len(single.Audio) != 1 || len(single.Videos) != 0 {
		t.Fatalf("single = %+v, %v", single, err)
	}
	// A carousel: each video has a code of its own, unlike the post's.
	carousel := []byte(reel("BQ0dSaohpPW", "one") + "\n" + reel("BQ0dTpOhuHT", "two") + "\n" + reel("BQ0dT7RBFeF", "three"))
	first, err := parseInspection(carousel, "BQ0eAlwhDrw", domain.SiteInstagram)
	if err != nil || len(first.Videos) != 3 || first.Videos[2].VideoID != "BQ0eAlwhDrw.3" || first.Videos[2].URL != "https://www.instagram.com/p/BQ0eAlwhDrw/?item=3" {
		t.Fatalf("carousel = %+v, %v", first, err)
	}
	third, err := parseInspection(carousel, "BQ0eAlwhDrw.3", domain.SiteInstagram)
	if err != nil || third.VideoID != "BQ0eAlwhDrw.3" || !strings.Contains(third.Thumbnail, "three") {
		t.Fatalf("third = %+v, %v", third, err)
	}
}

func TestInstagramFailures(t *testing.T) {
	for stderr, code := range map[string]string{
		"ERROR: [Instagram] CWqAgUZgCku: Instagram sent an empty media response. Check if this post is accessible": "signin_required",
		"ERROR: [Instagram] x: Requested content is not available, rate-limit reached or login required":           "instagram_limited",
		"ERROR: [Instagram] x: HTTP Error 404: Not Found":                                                          "video_unavailable",
		"ERROR: [Instagram] x: There is no video in this post":                                                     "video_unavailable",
	} {
		if err, ok := classifyFor(domain.SiteInstagram, stderr).(*Error); !ok || err.Code != code {
			t.Errorf("%s: %v", stderr, err)
		}
	}
}
