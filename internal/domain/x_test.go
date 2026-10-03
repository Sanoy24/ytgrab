package domain

import (
	"errors"
	"testing"
)

func TestXPostLinks(t *testing.T) {
	for link, id := range map[string]string{
		"https://x.com/DeadlineDayLive/status/2106271782382539167?s=20":   "2106271782382539167",
		"https://twitter.com/OpenAIDevs/status/2105708732323909827":       "2105708732323909827",
		"https://mobile.twitter.com/a/status/2106270671584313722/video/1": "2106270671584313722",
		"https://x.com/i/web/status/2106270671584313722":                  "2106270671584313722",
		"http://www.x.com/i/status/2106270671584313722":                   "2106270671584313722",
	} {
		url, videoID, err := ParseVideoURL(link)
		if err != nil || videoID != id || url != "https://x.com/i/status/"+id || SiteOf(url) != SiteX {
			t.Errorf("%s: %q %q %v", link, url, videoID, err)
		}
	}
	for _, bad := range []string{
		"https://x.com/DeadlineDayLive",
		"https://x.com/someone/status/abc",
		"https://x.com/someone/status/12",
		"https://notx.com/a/status/2106270671584313722",
		"https://x.com.evil.test/a/status/2106270671584313722",
	} {
		if _, _, err := ParseVideoURL(bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q accepted", bad)
		}
	}
	job, err := NewJob("https://x.com/OpenAIDevs/status/2105708732323909827", AudioM4A)
	if err != nil || job.Site != SiteX || job.VideoID != "2105708732323909827" {
		t.Fatalf("job = %+v, %v", job, err)
	}
	if yt, _ := NewJob("https://youtu.be/jNQXAC9IVRw", AudioM4A); yt.Site != SiteYouTube {
		t.Fatal("YouTube jobs keep an empty site")
	}
	if SafeThumbnail("https://pbs.twimg.com/media/HTkEkK4aoAAfGay.jpg?name=orig") == "" || SafeThumbnail("https://evil.test/x.jpg") != "" || SafeThumbnail("https://pbs.twimg.com/a\"onerror=") != "" {
		t.Error("SafeThumbnail")
	}
}
