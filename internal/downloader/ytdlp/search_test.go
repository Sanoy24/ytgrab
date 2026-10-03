package ytdlp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/config"
)

func TestParseSearch(t *testing.T) {
	data := []byte(`{"entries":[
		{"id":"jNQXAC9IVRw","title":"Me at the zoo","channel":"jawed","duration":19},
		{"id":"liveliveliv","title":"Live now","live_status":"is_live"},
		{"id":"not an id","title":"x"},
		{"id":"dQw4w9WgXcQ","title":"Song","uploader":"Rick Astley"}]}`)
	got, err := parseSearch(data)
	if err != nil || len(got) != 2 || got[0].Channel != "jawed" || got[1].Channel != "Rick Astley" {
		t.Fatalf("results = %+v, %v", got, err)
	}
}

func TestSearchRejectsBadQueriesBeforeRunningAnything(t *testing.T) {
	inspector := NewInspector(config.Config{})
	for _, q := range []string{"", " a ", strings.Repeat("x", 201)} {
		if _, err := inspector.Search(context.Background(), q); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%q: %v", q, err)
		}
	}
}
