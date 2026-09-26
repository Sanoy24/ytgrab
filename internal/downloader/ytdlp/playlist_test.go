package ytdlp

import (
	"fmt"
	"strings"
	"testing"

	"ytgrab/internal/config"
	"ytgrab/internal/domain"
)

func TestParsePlaylistSkipsUnavailableAndCaps(t *testing.T) {
	data := []byte(`{"id":"PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb","title":"Lectures","playlist_count":3,"entries":[
		{"_type":"url","id":"dQw4w9WgXcQ","title":"One","duration":213.0},
		{"_type":"url","id":"jNQXAC9IVRw","title":"[Private video]","duration":null},
		{"_type":"url","id":"aqz-KE-bpKQ","title":"[Deleted video]"},
		{"_type":"url","id":"bad id","title":"Broken"}]}`)
	list, err := parsePlaylist(data, "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb")
	if err != nil || list.Title != "Lectures" || len(list.Entries) != 1 || list.Entries[0].VideoID != "dQw4w9WgXcQ" || list.Unavailable != 3 || list.Truncated {
		t.Fatalf("playlist = %+v, %v", list, err)
	}

	var entries []string
	for i := range domain.MaxPlaylistItems + 1 {
		entries = append(entries, fmt.Sprintf(`{"id":"video%06d","title":"Video %d"}`, i, i))
	}
	big := []byte(`{"id":"PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb","title":"Big","playlist_count":120,"entries":[` + strings.Join(entries, ",") + `]}`)
	list, err = parsePlaylist(big, "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb")
	if err != nil || len(list.Entries) != domain.MaxPlaylistItems || !list.Truncated || list.Total == nil || *list.Total != 120 {
		t.Fatalf("big playlist = %d entries, truncated %v, total %v, %v", len(list.Entries), list.Truncated, list.Total, err)
	}

	if _, err := parsePlaylist([]byte(`{"id":"PLother00000","entries":[]}`), "PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb"); err == nil {
		t.Fatal("mismatched playlist ID accepted")
	}
}

func TestPlaylistArgumentsStayFlatAndBounded(t *testing.T) {
	args := strings.Join(playlistArgs("https://www.youtube.com/playlist?list=PLbpi6ZahtOH6Blw3RGYpWkSByi_T7Rygb"), " ")
	for _, want := range []string{"--flat-playlist", "--dump-single-json", fmt.Sprintf("--playlist-items 1:%d", domain.MaxPlaylistItems+1), "-- https://www.youtube.com/playlist?list="} {
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
