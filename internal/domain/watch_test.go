package domain

import (
	"errors"
	"testing"
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
