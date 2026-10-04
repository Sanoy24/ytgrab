package ytdlp

import (
	"testing"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

func TestParseListing(t *testing.T) {
	channel := []byte(`{"id":"UC_x5XG1OV2P6uZZ5FSM9Ttw","title":"Google for Developers - Videos","channel":"Google for Developers","entries":[
		{"id":"Rnz9mOyxk0k","title":"Newest"},{"id":"bad id","title":"x"},{"id":"s82Ho5HdltE","title":"[Private video]"},{"id":"-G431WC_-RM","title":"Older"}]}`)
	got, err := parseListing(channel, "channel", "")
	if err != nil || got.Title != "Google for Developers" || len(got.Entries) != 2 || got.Entries[0].VideoID != "Rnz9mOyxk0k" {
		t.Fatalf("channel = %+v, %v", got, err)
	}
	playlist := []byte(`{"id":"PLx","title":"My playlist","entries":[{"id":"aaaaaaaaaaa","title":"One"}]}`)
	if got, _ := parseListing(playlist, "playlist", ""); got.Title != "My playlist" || len(got.Entries) != 1 {
		t.Fatalf("playlist = %+v", got)
	}
}

func TestVimeoListing(t *testing.T) {
	channel := []byte(`{"id":"staffpicks","title":"Vimeo Staff Picks","entries":[
		{"id":"1228694119","title":"Anonymity &mdash; Maison Margiela"},
		{"id":"jNQXAC9IVRw","title":"not a Vimeo ID"}]}`)
	got, err := parseListing(channel, "channel", domain.SiteVimeo)
	if err != nil || got.Title != "Vimeo Staff Picks" || len(got.Entries) != 1 || got.Entries[0].Title != "Anonymity — Maison Margiela" {
		t.Fatalf("channel = %+v, %v", got, err)
	}
	// Showcases list their videos without titles; they are still videos.
	showcase := []byte(`{"id":"2632481","title":"Staff Favorites","entries":[{"id":"79761619","title":null},{"id":"79695097"}]}`)
	if got, _ := parseListing(showcase, "playlist", domain.SiteVimeo); len(got.Entries) != 2 {
		t.Fatalf("showcase = %+v", got)
	}
}
