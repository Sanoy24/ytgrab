package ytdlp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestVimeoArguments(t *testing.T) {
	video, err := domain.NewJob("https://vimeo.com/22439234", domain.Video720)
	if err != nil {
		t.Fatal(err)
	}
	args := buildArgs(video, config.Config{SponsorBlock: "remove"})
	if joined := strings.Join(args, " "); !strings.HasSuffix(joined, "-f bv*[height<=720]+ba/b[height<=720] --merge-output-format mp4/mkv --merge-output-format mp4 -- https://player.vimeo.com/video/22439234") || strings.Contains(joined, "sponsorblock") {
		t.Errorf("args = %s", joined)
	}
	if got := argAfter(args, "-o"); got != "%(title).150B [22439234] %(width)sx%(height)s.%(ext)s" {
		t.Errorf("name = %q", got)
	}
	audio, _ := domain.NewJob("https://vimeo.com/22439234", domain.AudioM4A)
	if joined := strings.Join(buildArgs(audio, config.Config{}), " "); !strings.Contains(joined, "-f ba[ext=m4a]/ba -x --audio-format m4a") {
		t.Errorf("M4A args = %s", joined)
	}
	picked, _ := domain.NewFormatJob("https://vimeo.com/22439234", domain.FormatSelection{Kind: "audio", ID: "hls-akfire_interconnect_quic-audio-high-English", Ext: "mp4", Label: "Audio"})
	if joined := strings.Join(buildArgs(picked, config.Config{}), " "); !strings.Contains(joined, "-f hls-akfire_interconnect_quic-audio-high-English -x --audio-format m4a") {
		t.Errorf("picked audio args = %s", joined)
	}
	// YouTube keeps picking M4A audio as before.
	youtube, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	if joined := strings.Join(buildArgs(youtube, config.Config{}), " "); !strings.Contains(joined, "-f ba[ext=m4a] --") {
		t.Errorf("YouTube M4A args = %s", joined)
	}
}

func TestVimeoInspection(t *testing.T) {
	data := []byte(`{"id":"22439234","display_id":"22439234","title":"The Mountain","duration":185,
		"thumbnail":"https://i.vimeocdn.com/video/145027281-cf3e3e047a52_1280",
		"formats":[
			{"format_id":"hls-akfire_interconnect_quic-audio-low-English","ext":"mp4","vcodec":"none","protocol":"m3u8_native"},
			{"format_id":"hls-akfire_interconnect_quic-audio-high-English","ext":"mp4","vcodec":"none","protocol":"m3u8_native"},
			{"format_id":"hls-akfire_interconnect_quic-5438","ext":"mp4","vcodec":"avc1.64002A","acodec":"none","width":1920,"height":1080,"protocol":"m3u8_native","tbr":5438}]}`)
	got, err := parseInspection(data, "22439234", domain.SiteVimeo)
	if err != nil || got.Site != "vimeo" || got.Thumbnail == "" || len(got.Video) != 1 || len(got.Audio) != 2 || got.Audio[0].ID != "hls-akfire_interconnect_quic-audio-high-English" || got.Audio[0].Ext != "m4a" {
		t.Fatalf("inspection = %+v, %v", got, err)
	}
	if size := got.Video[0].FileSizeApprox; size == nil || *size != estimate(5438, 185) {
		t.Errorf("size = %v", size)
	}
}

func TestVimeoFailures(t *testing.T) {
	for stderr, code := range map[string]string{
		"ERROR: [vimeo] 1: The web client only works when logged-in":                      "signin_required",
		"ERROR: [vimeo:player] 1: Cannot download embed-only video without embedding URL": "video_unavailable",
		"ERROR: [vimeo] 1: HTTP Error 404: Not Found":                                     "video_unavailable",
		"ERROR: [vimeo] 1: HTTP Error 429: Too Many Requests":                             "vimeo_limited",
		"ERROR: This format is DRM protected; Try selecting another format":               "drm_protected",
	} {
		if err, ok := classifyFor(domain.SiteVimeo, stderr).(*Error); !ok || err.Code != code {
			t.Errorf("%s: %v", stderr, err)
		}
	}
}

func TestRemoveThumbnails(t *testing.T) {
	dir := t.TempDir()
	keep := []string{"Talk [22439234].mp4.part", "Other [12345678].jpg", "Talk [22439234].mp3", "cover.jpg"}
	gone := []string{"Talk [22439234].jpg", "Talk [22439234].webp"}
	for _, name := range append(keep, gone...) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	removeThumbnails(dir, "22439234")
	for _, name := range gone {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s was kept", name)
		}
	}
	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s was removed", name)
		}
	}
}
