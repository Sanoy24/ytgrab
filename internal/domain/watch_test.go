package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWatchLinks(t *testing.T) {
	good := map[string][2]string{
		"https://www.youtube.com/@GoogleDevelopers":                                           {"channel", "https://www.youtube.com/@GoogleDevelopers/videos"},
		"youtube.com/@GoogleDevelopers/shorts":                                                {"channel", "https://www.youtube.com/@GoogleDevelopers/videos"},
		"https://m.youtube.com/channel/UC_x5XG1OV2P6uZZ5FSM9Ttw/featured":                     {"channel", "https://www.youtube.com/channel/UC_x5XG1OV2P6uZZ5FSM9Ttw/videos"},
		"https://www.youtube.com/c/GoogleDevelopers":                                          {"channel", "https://www.youtube.com/c/GoogleDevelopers/videos"},
		"https://www.youtube.com/user/GoogleDevelopers":                                       {"channel", "https://www.youtube.com/user/GoogleDevelopers/videos"},
		"https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2":            {"playlist", "https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2"},
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2": {"playlist", "https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2"},
	}
	for link, want := range good {
		watch, err := NewWatch(link, AudioM4A)
		if err != nil || watch.Kind != want[0] || watch.URL != want[1] {
			t.Errorf("%s: %+v, %v", link, watch, err)
		}
	}
	for _, bad := range []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ", // a single video
		"https://vimeo.com/@someone",
		"https://www.youtube.com/@x",                      // handle too short
		"https://www.youtube.com/@name;rm -rf",            // not a handle
		"https://www.youtube.com/channel/UCnotachannelid", // wrong length
		"https://www.youtube.com/results?search_query=x",
		"",
	} {
		if _, err := NewWatch(bad, AudioM4A); !errors.Is(err, ErrInvalidWatchURL) {
			t.Errorf("%q accepted: %v", bad, err)
		}
	}
	if _, err := NewWatch("https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=RDdQw4w9WgXcQ", AudioM4A); !errors.Is(err, ErrMixPlaylist) {
		t.Errorf("mix accepted: %v", err)
	}
	if _, err := NewWatch("https://www.youtube.com/@GoogleDevelopers", "audio-ogg"); !errors.Is(err, ErrInvalidPreset) {
		t.Errorf("bad preset accepted: %v", err)
	}
}

func TestWatchFilters(t *testing.T) {
	minutes := func(m float64) *float64 { s := m * 60; return &s }
	w := Watch{MinMinutes: 2, Keywords: "Gemma, android "}
	for _, c := range []struct {
		title    string
		duration *float64
		want     bool
	}{
		{"Gemma 4 explained", minutes(10), true},
		{"What's new in ANDROID", minutes(3), true},
		{"Gemma short", minutes(1), false},          // too short (a Short)
		{"Chrome DevTools tips", minutes(9), false}, // no keyword
		{"gemma, no length listed", nil, true},
	} {
		if got := w.Wants(c.title, c.duration); got != c.want {
			t.Errorf("%q: %v, want %v", c.title, got, c.want)
		}
	}
	if !(Watch{}).Wants("anything", minutes(0.2)) {
		t.Error("a watch without filters wants everything")
	}
	if (Watch{}).Interval() != 6*time.Hour || (Watch{IntervalHours: 1}).Interval() != time.Hour {
		t.Error("intervals")
	}
	if ValidWatchOptions(0, "", 3) || ValidWatchOptions(-1, "", 6) || ValidWatchOptions(0, strings.Repeat("x", 201), 6) || !ValidWatchOptions(5, "a,b", 24) {
		t.Error("ValidWatchOptions")
	}
}

func TestVimeoWatchLinks(t *testing.T) {
	for link, want := range map[string][2]string{
		"https://vimeo.com/channels/staffpicks":          {"channel", "https://vimeo.com/channels/staffpicks"},
		"vimeo.com/channels/staffpicks/1231817291":       {"channel", "https://vimeo.com/channels/staffpicks"},
		"https://vimeo.com/groups/motion":                {"channel", "https://vimeo.com/groups/motion/videos"},
		"https://www.vimeo.com/groups/motion/videos?x=1": {"channel", "https://vimeo.com/groups/motion/videos"},
		"https://vimeo.com/showcase/7064593":             {"playlist", "https://vimeo.com/showcase/7064593"},
		"https://vimeo.com/album/2632481":                {"playlist", "https://vimeo.com/showcase/2632481"},
	} {
		watch, err := NewWatch(link, AudioM4A)
		if err != nil || watch.Kind != want[0] || watch.URL != want[1] || watch.Site != SiteVimeo {
			t.Errorf("%s: %+v %v", link, watch, err)
		}
	}
	if _, err := NewWatch("https://vimeo.com/someone", AudioM4A); !errors.Is(err, ErrVimeoPeople) {
		t.Errorf("person's page = %v", err)
	}
	for _, bad := range []string{"https://vimeo.com/76979871", "https://vimeo.com/channels/", "https://vimeo.com/showcase/abc"} {
		if _, err := NewWatch(bad, AudioM4A); !errors.Is(err, ErrInvalidWatchURL) {
			t.Errorf("%q = %v", bad, err)
		}
	}
	if yt, _ := NewWatch("https://www.youtube.com/@GoogleDevelopers", AudioM4A); yt.Site != SiteYouTube {
		t.Error("YouTube watches keep an empty site")
	}
	if WatchVideoURL(SiteVimeo, "1231817291") != "https://player.vimeo.com/video/1231817291" || WatchVideoURL("", "jNQXAC9IVRw") != "https://www.youtube.com/watch?v=jNQXAC9IVRw" {
		t.Error("WatchVideoURL")
	}
	if !ValidWatchVideoID(SiteVimeo, "1231817291") || ValidWatchVideoID(SiteVimeo, "jNQXAC9IVRw") || !ValidWatchVideoID("", "jNQXAC9IVRw") {
		t.Error("ValidWatchVideoID")
	}
}
