package ytdlp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestMediaServerLayout(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.Video720)
	args := buildArgs(job, config.Config{FileNames: MediaServerStyle})
	want := "%(channel,uploader|Unknown channel).60B/Season %(upload_date>%Y|0000)s/%(channel,uploader|Unknown channel).60B - S%(upload_date>%Y|0000)sE%(upload_date>%m%d|0000)s - %(title).150B [%(id)s] %(height)sp.%(ext)s"
	if got := argAfter(args, "-o"); got != want {
		t.Errorf("name = %q", got)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-o thumbnail:"+strings.TrimSuffix(want, ".%(ext)s")+"-thumb.%(ext)s") || !strings.Contains(joined, "after_move:"+metaPrefix) {
		t.Errorf("args = %s", joined)
	}
	// Audio doesn't belong in a TV library's layout extras.
	audio, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	if joined := strings.Join(buildArgs(audio, config.Config{FileNames: MediaServerStyle}), " "); strings.Contains(joined, metaPrefix) {
		t.Errorf("audio args = %s", joined)
	}
}

func TestNFOFiles(t *testing.T) {
	season := filepath.Join(t.TempDir(), "Tom & Jerry", "Season 2005")
	if err := os.MkdirAll(season, 0o700); err != nil {
		t.Fatal(err)
	}
	video := filepath.Join(season, "Tom & Jerry - S2005E0423 - Me at the zoo [jNQXAC9IVRw] 240p.mp4")
	minutes := 19.0
	meta := Metadata{ID: "jNQXAC9IVRw", Title: "Me at the <zoo>", Channel: "Tom & Jerry", ChannelID: "UC4QobU6STFB0P71PMvOGN5A", UploadDate: "20050423", Description: "The first video & more", Duration: &minutes}
	if err := writeNFOs(video, meta, ""); err != nil {
		t.Fatal(err)
	}
	episode, _ := os.ReadFile(strings.TrimSuffix(video, ".mp4") + ".nfo")
	for _, want := range []string{"<episodedetails>", "<title>Me at the &lt;zoo&gt;</title>", "<showtitle>Tom &amp; Jerry</showtitle>", "<season>2005</season>", "<episode>0423</episode>", "<aired>2005-04-23</aired>", `<uniqueid type="youtube" default="true">jNQXAC9IVRw</uniqueid>`} {
		if !strings.Contains(string(episode), want) {
			t.Errorf("episode .nfo lacks %s:\n%s", want, episode)
		}
	}
	show := filepath.Join(filepath.Dir(season), "tvshow.nfo")
	data, _ := os.ReadFile(show)
	if !strings.Contains(string(data), "<title>Tom &amp; Jerry</title>") || !strings.Contains(string(data), "UC4QobU6STFB0P71PMvOGN5A") {
		t.Errorf("tvshow.nfo:\n%s", data)
	}
	// A tvshow.nfo the user edited is kept.
	if err := os.WriteFile(show, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeNFOs(video, meta, ""); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(show); string(data) != "mine" {
		t.Errorf("tvshow.nfo was replaced: %s", data)
	}
}
