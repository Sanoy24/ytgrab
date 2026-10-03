package ytdlp

import "testing"

func TestParseListing(t *testing.T) {
	channel := []byte(`{"id":"UC_x5XG1OV2P6uZZ5FSM9Ttw","title":"Google for Developers - Videos","channel":"Google for Developers","entries":[
		{"id":"Rnz9mOyxk0k","title":"Newest"},{"id":"bad id","title":"x"},{"id":"s82Ho5HdltE","title":"[Private video]"},{"id":"-G431WC_-RM","title":"Older"}]}`)
	got, err := parseListing(channel, "channel")
	if err != nil || got.Title != "Google for Developers" || len(got.Entries) != 2 || got.Entries[0].VideoID != "Rnz9mOyxk0k" {
		t.Fatalf("channel = %+v, %v", got, err)
	}
	playlist := []byte(`{"id":"PLx","title":"My playlist","entries":[{"id":"aaaaaaaaaaa","title":"One"}]}`)
	if got, _ := parseListing(playlist, "playlist"); got.Title != "My playlist" || len(got.Entries) != 1 {
		t.Fatalf("playlist = %+v", got)
	}
}
