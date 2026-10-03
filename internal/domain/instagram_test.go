package domain

import (
	"errors"
	"testing"
)

func TestInstagramLinks(t *testing.T) {
	for link, want := range map[string][2]string{
		"https://www.instagram.com/reel/Dd_8Q80veb1/?utm_source=ig_web_copy_link&stkn=NTc4": {"https://www.instagram.com/p/Dd_8Q80veb1/", "Dd_8Q80veb1"},
		"https://instagram.com/p/BQ0eAlwhDrw/?img_index=2":                                  {"https://www.instagram.com/p/BQ0eAlwhDrw/", "BQ0eAlwhDrw"},
		"https://www.instagram.com/someone/reel/DeAVy0uTbPn/":                               {"https://www.instagram.com/p/DeAVy0uTbPn/", "DeAVy0uTbPn"},
		"https://www.instagram.com/tv/aye83DjauH":                                           {"https://www.instagram.com/p/aye83DjauH/", "aye83DjauH"},
	} {
		url, videoID, err := ParseVideoURL(link)
		if err != nil || url != want[0] || videoID != want[1] || SiteOf(url) != SiteInstagram {
			t.Errorf("%s: %q %q %v", link, url, videoID, err)
		}
	}
	for _, bad := range []string{
		"https://www.instagram.com/someone/",
		"https://www.instagram.com/stories/someone/123/",
		"https://www.instagram.com/p/a.b/",
		"https://instagram.com.evil.test/p/Dd_8Q80veb1/",
	} {
		if _, _, err := ParseVideoURL(bad); !errors.Is(err, ErrInvalidURL) {
			t.Errorf("%q accepted", bad)
		}
	}
	if post, index := PostItem(InstagramItemURL("Dd_8Q80veb1", 3)); post != "https://www.instagram.com/p/Dd_8Q80veb1/" || index != 3 {
		t.Errorf("PostItem = %q %d", post, index)
	}
	for id, want := range map[string]struct {
		post  string
		index int
	}{"Dd_8Q80veb1": {"Dd_8Q80veb1", 1}, "Dd-8Q80veb1.2": {"Dd-8Q80veb1", 2}} {
		if post, index := SplitItemID(SiteInstagram, id); post != want.post || index != want.index {
			t.Errorf("SplitItemID(%q) = %q %d", id, post, index)
		}
	}
	if post, index := SplitItemID(SiteX, "1600649710662213632-2"); post != "1600649710662213632" || index != 2 {
		t.Errorf("X SplitItemID = %q %d", post, index)
	}
	for link, ok := range map[string]bool{
		"https://instagram.fadd1-1.fna.fbcdn.net/v/t51.82787-15/830715575_n.jpg?stp=dst&_nc_ht=x": true,
		"https://scontent.cdninstagram.com/v/a.jpg":                                               true,
		"https://fbcdn.net.evil.test/a.jpg":                                                       false,
		"http://instagram.fadd1-1.fna.fbcdn.net/a.jpg":                                            false,
		"https://instagram.fadd1-1.fna.fbcdn.net:8443/a.jpg":                                      false,
	} {
		if (SafeThumbnail(link) != "") != ok {
			t.Errorf("SafeThumbnail(%q) kept = %v", link, !ok)
		}
	}
}
