package domain

import (
	"errors"
	"testing"
)

func TestRedditLinks(t *testing.T) {
	for link, want := range map[string][2]string{
		"https://www.reddit.com/r/videos/comments/6rrwyj/that_small_heart_attack/": {"https://www.reddit.com/comments/6rrwyj", "6rrwyj"},
		"https://old.reddit.com/r/videos/comments/6rrwyj/":                         {"https://www.reddit.com/comments/6rrwyj", "6rrwyj"},
		"https://reddit.com/comments/6rrwyj":                                       {"https://www.reddit.com/comments/6rrwyj", "6rrwyj"},
		"https://redd.it/6rrwyj":                                                   {"https://www.reddit.com/comments/6rrwyj", "6rrwyj"},
		"https://v.redd.it/zv89llsvexdz":                                           {"https://v.redd.it/zv89llsvexdz", "zv89llsvexdz"},
	} {
		url, videoID, err := ParseVideoURL(link)
		if err != nil || url != want[0] || videoID != want[1] || SiteOf(url) != SiteReddit {
			t.Errorf("%s: %q %q %v", link, url, videoID, err)
		}
	}
	for _, bad := range []string{
		"https://www.reddit.com/r/videos/",
		"https://www.reddit.com/r/videos/s/AbCdEf1234",
		"https://www.reddit.com/r/videos/comments/NOT-AN-ID/",
		"https://reddit.com.evil.test/r/videos/comments/6rrwyj/",
	} {
		if _, _, err := ParseVideoURL(bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q accepted", bad)
		}
	}
	if SafeThumbnail("https://external-preview.redd.it/cb9n.png?format=pjpg&auto=webp&s=2fea") == "" || SafeThumbnail("https://evil.test/a.png") != "" {
		t.Error("SafeThumbnail")
	}
}
