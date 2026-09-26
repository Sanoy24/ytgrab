package app

import (
	"reflect"
	"testing"
)

func TestBrowserCommandUsesFixedURL(t *testing.T) {
	const url = "http://127.0.0.1:8787/"
	for goos, want := range map[string][]string{
		"windows": {"rundll32", "url.dll,FileProtocolHandler", url},
		"darwin":  {"open", url},
		"linux":   {"xdg-open", url},
	} {
		if got := browserCommand(goos, url); !reflect.DeepEqual(got, want) {
			t.Errorf("browserCommand(%s) = %v, want %v", goos, got, want)
		}
	}
}
