package domain

import (
	"errors"
	"testing"
)

func TestVimeoLinks(t *testing.T) {
	for link, want := range map[string][2]string{
		"https://vimeo.com/76979871":                            {"https://player.vimeo.com/video/76979871", "76979871"},
		"https://www.vimeo.com/22439234?share=copy":             {"https://player.vimeo.com/video/22439234", "22439234"},
		"https://vimeo.com/123456789/abcdef1234":                {"https://player.vimeo.com/video/123456789?h=abcdef1234", "123456789"},
		"https://player.vimeo.com/video/123456789?h=abcdef1234": {"https://player.vimeo.com/video/123456789?h=abcdef1234", "123456789"},
		"https://vimeo.com/channels/staffpicks/76979871":        {"https://player.vimeo.com/video/76979871", "76979871"},
		"https://vimeo.com/groups/shortfilms/videos/76979871":   {"https://player.vimeo.com/video/76979871", "76979871"},
		"https://vimeo.com/showcase/1234567/video/76979871":     {"https://player.vimeo.com/video/76979871", "76979871"},
	} {
		url, videoID, err := ParseVideoURL(link)
		if err != nil || url != want[0] || videoID != want[1] || SiteOf(url) != SiteVimeo {
			t.Errorf("%s: %q %q %v", link, url, videoID, err)
		}
	}
	for _, bad := range []string{
		"https://vimeo.com/staffpicks",
		"https://vimeo.com/showcase/1234567",
		"https://vimeo.com/123456789/not-a-hash!",
		"https://player.vimeo.com/video/abc",
		"https://vimeo.com.evil.test/76979871",
	} {
		if _, _, err := ParseVideoURL(bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q accepted", bad)
		}
	}
	if SafeThumbnail("https://i.vimeocdn.com/video/452001751-8216e057_640") == "" {
		t.Error("Vimeo thumbnail refused")
	}
}
