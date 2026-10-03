package ytdlp

import (
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

const redditPost = "https://www.reddit.com/r/videos/comments/6rrwyj/that_small_heart_attack/"

func TestRedditArguments(t *testing.T) {
	job, err := domain.NewJob(redditPost, domain.Video720)
	if err != nil {
		t.Fatal(err)
	}
	args := buildArgs(job, config.Config{SponsorBlock: "remove", SubtitlesMode: "embed", SubtitlesLang: "en"})
	joined := strings.Join(args, " ")
	if !strings.HasSuffix(joined, "-f bv*[height<=720]+ba/b[height<=720] --merge-output-format mp4/mkv -- https://www.reddit.com/comments/6rrwyj") {
		t.Errorf("args = %s", joined)
	}
	if strings.Contains(joined, "sponsorblock") || strings.Contains(joined, "--write-subs") {
		t.Errorf("YouTube-only options used for Reddit: %s", joined)
	}
	if got := argAfter(args, "-o"); got != "%(title).150B [6rrwyj] %(width)sx%(height)s.%(ext)s" {
		t.Errorf("name = %q", got)
	}
}

func TestRedditInspection(t *testing.T) {
	data := []byte(`{"id":"zv89llsvexdz","display_id":"6rrwyj","title":"That small heart attack &amp; more","duration":12,
		"thumbnail":"https://external-preview.redd.it/cb9n.png?format=pjpg&auto=webp&s=2fea",
		"formats":[
			{"format_id":"hls-audio-0-Default","ext":"mp4","vcodec":"none","protocol":"m3u8_native"},
			{"format_id":"dash-AUDIO-1","ext":"m4a","vcodec":"none","acodec":"mp4a.40.2","protocol":"https","tbr":131.111,"abr":131.111},
			{"format_id":"dash-VIDEO-1","ext":"mp4","vcodec":"avc1.4d401f","acodec":"none","width":264,"height":480,"protocol":"https","tbr":2416.183}]}`)
	got, err := parseInspection(data, "6rrwyj", domain.SiteReddit)
	if err != nil || got.Site != "reddit" || got.Title != "That small heart attack & more" || got.Thumbnail == "" || len(got.Video) != 1 || len(got.Audio) != 1 {
		t.Fatalf("inspection = %+v, %v", got, err)
	}
	if size := got.Video[0].FileSizeApprox; size == nil || *size != estimate(2416.183, 12) {
		t.Errorf("video size = %v", size)
	}
	// A v.redd.it link is matched by the video's own ID.
	if _, err := parseInspection(data, "zv89llsvexdz", domain.SiteReddit); err != nil {
		t.Error(err)
	}
	if _, err := parseInspection(data, "abcdef", domain.SiteReddit); err == nil {
		t.Error("a different post was accepted")
	}
}

func TestRedditFailures(t *testing.T) {
	for stderr, code := range map[string]string{
		"ERROR: [Reddit] 6rrwyj: No media found":                    "video_unavailable",
		"ERROR: [Reddit] 6rrwyj: HTTP Error 403: Forbidden":         "signin_required",
		"ERROR: [Reddit] 6rrwyj: HTTP Error 429: Too Many Requests": "reddit_limited",
	} {
		if err, ok := classifyFor(domain.SiteReddit, stderr).(*Error); !ok || err.Code != code {
			t.Errorf("%s: %v", stderr, err)
		}
	}
}

// Converting Reddit's preview image fails (it claims to be PNG but isn't), which made
// yt-dlp fail an otherwise finished audio download.
func TestRedditAudioHasNoCoverArt(t *testing.T) {
	picked, err := domain.NewFormatJob(redditPost, domain.FormatSelection{Kind: "audio", ID: "dash-AUDIO-1", Ext: "m4a", Label: "Audio · M4A 131 kbps"})
	if err != nil {
		t.Fatal(err)
	}
	preset, _ := domain.NewJob(redditPost, domain.AudioMP3)
	for _, job := range []domain.Job{picked, preset} {
		if args := strings.Join(buildArgs(job, config.Config{}), " "); strings.Contains(args, "--embed-thumbnail") {
			t.Errorf("cover art embedded for Reddit: %s", args)
		}
	}
}
