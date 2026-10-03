package ytdlp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/cooldown"
	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestParsePlaylistSkipsUnavailableAndCaps(t *testing.T) {
	data := []byte(`{"id":"PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb","title":"Lectures","playlist_count":3,"entries":[
		{"_type":"url","id":"dQw4w9WgXcQ","title":"One","duration":213.0},
		{"_type":"url","id":"jNQXAC9IVRw","title":"[Private video]","duration":null},
		{"_type":"url","id":"aqz-KE-bpKQ","title":"[Deleted video]"},
		{"_type":"url","id":"bad id","title":"Broken"},
		{"_type":"url","id":"nTu8Ew4jsmE","title":null,"duration":null},
		{"_type":"url","id":"6sx0zTiTO3k","title":"[Unavailable video]"}]}`)
	list, err := parsePlaylist(data, "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", 1)
	if err != nil || list.Title != "Lectures" || len(list.Entries) != 1 || list.Entries[0].VideoID != "dQw4w9WgXcQ" || list.Unavailable != 5 || list.Truncated {
		t.Fatalf("playlist = %+v, %v", list, err)
	}

	var entries []string
	for i := range domain.PlaylistPageSize + 1 {
		entries = append(entries, fmt.Sprintf(`{"id":"video%06d","title":"Video %d"}`, i, i))
	}
	big := []byte(`{"id":"PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb","title":"Big","playlist_count":120,"entries":[` + strings.Join(entries, ",") + `]}`)
	list, err = parsePlaylist(big, "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", 1)
	if err != nil || len(list.Entries) != domain.PlaylistPageSize || !list.Truncated || list.Next == nil || *list.Next != 51 || list.Total == nil || *list.Total != 120 {
		t.Fatalf("big playlist = %d entries, truncated %v, total %v, %v", len(list.Entries), list.Truncated, list.Total, err)
	}

	if _, err := parsePlaylist([]byte(`{"id":"PLother00000","entries":[]}`), "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", 1); err == nil {
		t.Fatal("mismatched playlist ID accepted")
	}
}

func TestPlaylistArgumentsStayFlatAndBounded(t *testing.T) {
	args := strings.Join(playlistArgs("https://www.youtube.com/playlist?list=PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", 1), " ")
	for _, want := range []string{"--flat-playlist", "--dump-single-json", fmt.Sprintf("--playlist-items 1:%d", domain.PlaylistPageSize+1), "-- https://www.youtube.com/playlist?list="} {
		if !strings.Contains(args, want) {
			t.Errorf("playlist args missing %q: %s", want, args)
		}
	}
}

func TestListedTitlesAreRemembered(t *testing.T) {
	inspector := NewInspector(config.Config{})
	inspector.rememberTitles(Playlist{Entries: []PlaylistEntry{{VideoID: "dQw4w9WgXcQ", Title: "One"}}})
	if got := inspector.CachedTitle("dQw4w9WgXcQ"); got != "One" {
		t.Fatalf("CachedTitle = %q", got)
	}
	if got := inspector.CachedTitle("jNQXAC9IVRw"); got != "" {
		t.Fatalf("unknown title = %q", got)
	}
}

func TestMissingPlaylistIsUnavailable(t *testing.T) {
	err, ok := classifyFailure("ERROR: [youtube:tab] PLx: YouTube said: The playlist does not exist.").(*Error)
	if !ok || err.Code != "video_unavailable" {
		t.Fatalf("classifyFailure = %v", err)
	}
}

func TestChecksWaitDuringCooldown(t *testing.T) {
	inspector := NewInspector(config.Config{})
	inspector.Cooldown = cooldown.WithSteps(30 * time.Minute)
	inspector.Cooldown.Block()
	for name, check := range map[string]func() error{
		"inspect": func() error {
			_, err := inspector.Inspect(context.Background(), "https://youtu.be/jNQXAC9IVRw")
			return err
		},
		"playlist": func() error {
			_, err := inspector.ListPlaylist(context.Background(), "https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2", 1)
			return err
		},
	} {
		err := check()
		var toolError *Error
		// Without a configured yt-dlp, running it would report dependency_missing instead.
		if !errors.As(err, &toolError) || toolError.Code != "blocked" || !strings.Contains(toolError.Message, "about 30 min") {
			t.Errorf("%s during cooldown = %v", name, err)
		}
	}
}

func TestDownloadsAndListingsPaceRequests(t *testing.T) {
	job, _ := domain.NewJob("https://youtu.be/jNQXAC9IVRw", domain.AudioM4A)
	for name, args := range map[string][]string{
		"download": buildArgs(job, config.Config{}),
		"playlist": playlistArgs("https://www.youtube.com/playlist?list=PLav47HAVZMjnTdm25KnxGkL8e1sPRt8A2", 1),
	} {
		if !strings.Contains(strings.Join(args, " "), "--sleep-requests 0.5") {
			t.Errorf("%s arguments do not pace requests: %v", name, args)
		}
	}
}

func TestLaterPlaylistPages(t *testing.T) {
	if got := strings.Join(playlistArgs("https://www.youtube.com/playlist?list=PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", 51), " "); !strings.Contains(got, "--playlist-items 51:101") {
		t.Fatalf("second page args = %s", got)
	}
	var entries []string
	for i := range 30 { // the last page of an 80-video playlist
		entries = append(entries, fmt.Sprintf(`{"id":"video%06d","title":"Video %d"}`, 51+i, 51+i))
	}
	data := []byte(`{"id":"PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb","title":"Big","playlist_count":80,"entries":[` + strings.Join(entries, ",") + `]}`)
	list, err := parsePlaylist(data, "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb", 51)
	if err != nil || list.Start != 51 || len(list.Entries) != 30 || list.Next != nil || list.Truncated {
		t.Fatalf("last page = start %d, %d entries, next %v, truncated %v, %v", list.Start, len(list.Entries), list.Next, list.Truncated, err)
	}
}
