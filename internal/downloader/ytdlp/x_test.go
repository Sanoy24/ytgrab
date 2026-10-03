package ytdlp

import (
	"slices"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

const xPost = "https://x.com/Ksparth12/status/2106270671584313722"

func TestXArguments(t *testing.T) {
	for preset, want := range map[domain.Preset][]string{
		domain.VideoBest: {"-f", "b"},
		domain.Video720:  {"-f", "b", "-S", "res:720"},
		domain.AudioM4A:  {"-f", "ba/b", "-x", "--audio-format", "m4a"},
		domain.AudioMP3:  {"-f", "ba/b", "-x", "--audio-format", "mp3", "--audio-quality", "0"},
	} {
		job, err := domain.NewJob(xPost, preset)
		if err != nil {
			t.Fatal(err)
		}
		args := buildArgs(job, config.Config{SponsorBlock: "remove", SubtitlesMode: "embed", SubtitlesLang: "en"})
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, strings.Join(want, " ")+" -- https://x.com/i/status/2106270671584313722") {
			t.Errorf("%s: args end %v", preset, args[len(args)-10:])
		}
		if strings.Contains(joined, "sponsorblock") || strings.Contains(joined, "--write-subs") || strings.Contains(joined, "merge-output") {
			t.Errorf("%s: YouTube-only options used for X: %s", preset, joined)
		}
		if argAfter(args, "--replace-in-metadata") != "title" {
			t.Errorf("%s: HTML entities not cleaned from titles", preset)
		}
	}
	video, _ := domain.NewJob(xPost, domain.Video720)
	if got := argAfter(buildArgs(video, config.Config{}), "-o"); got != "%(title).150B [%(display_id)s] %(width)sx%(height)s.%(ext)s" {
		t.Errorf("video name = %q", got)
	}
	audio, _ := domain.NewJob(xPost, domain.AudioM4A)
	if got := argAfter(buildArgs(audio, config.Config{}), "-o"); got != "%(title).150B [%(display_id)s].%(ext)s" {
		t.Errorf("audio name = %q", got)
	}
	picked, _ := domain.NewFormatJob(xPost, domain.FormatSelection{Kind: "video", ID: "http-2176", Label: "Video · 720p"})
	if args := buildArgs(picked, config.Config{}); argAfter(args, "-f") != "http-2176" || slices.Contains(args, "--merge-output-format") {
		t.Errorf("picked format args = %v", args)
	}
}

func TestXInspection(t *testing.T) {
	data := []byte(`{"id":"2106270368000606208","display_id":"2106270671584313722","title":"Parth Sharma - Reintroducing WensityUI","duration":64.277,
		"thumbnail":"https://pbs.twimg.com/amplify_video_thumb/2106270368000606208/img/x.jpg?name=orig",
		"formats":[
		{"format_id":"hls-audio-128000-Audio","ext":"mp4","vcodec":"none","protocol":"m3u8_native","tbr":128},
		{"format_id":"hls-3274","ext":"mp4","vcodec":"avc1.640032","acodec":"none","width":1920,"height":1080,"protocol":"m3u8_native","tbr":3274},
		{"format_id":"http-832","ext":"mp4","width":640,"height":360,"protocol":"https","tbr":832},
		{"format_id":"hls-381","ext":"mp4","vcodec":"avc1.4D401F","acodec":"none","width":640,"height":360,"protocol":"m3u8_native"},
		{"format_id":"http-10368","ext":"mp4","width":1920,"height":1080,"protocol":"https","tbr":10368,"filesize_approx":83305000}]}`)
	got, err := parseInspection(data, "2106270671584313722", domain.SiteX)
	if err != nil || got.Site != "x" || len(got.Video) != 2 || len(got.Audio) != 0 || got.Thumbnail == "" {
		t.Fatalf("inspection = %+v, %v", got, err)
	}
	// 1080p: the streamed copy's 3274 kbps plus 128 kbps of audio, not the MP4's nominal 10368.
	if got.Video[1].FileSizeApprox == nil || *got.Video[1].FileSizeApprox != estimate(3274+128, 64.277) {
		t.Errorf("size estimate = %v", got.Video[1].FileSizeApprox)
	}
	if _, err := parseInspection(data, "999999999999", domain.SiteX); err == nil {
		t.Error("a different post was accepted")
	}
}

func TestXFailuresDontPauseYouTube(t *testing.T) {
	for stderr, code := range map[string]string{
		"ERROR: [twitter] 123: NSFW tweet requires authentication":    "signin_required",
		"ERROR: [twitter] 123: No video could be found in this tweet": "video_unavailable",
		"ERROR: [twitter] 123: HTTP Error 429: Too Many Requests":     "x_limited",
		"ERROR: [twitter] 123: Account suspended":                     "video_unavailable",
	} {
		err, _ := classifyFor(domain.SiteX, stderr).(*Error)
		if err == nil || err.Code != code {
			t.Errorf("%q -> %v, want %s", stderr, err, code)
		}
	}
	if err, _ := classifyFor(domain.SiteYouTube, "HTTP Error 429: Too Many Requests").(*Error); err.Code != "blocked" {
		t.Error("YouTube's 429 must still pause YouTube downloads")
	}
}

func estimate(kbps, seconds float64) int64 { return int64(kbps * seconds * 125) }
