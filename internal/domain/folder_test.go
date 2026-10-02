package domain

import (
	"strings"
	"testing"
)

func TestSafeFolderName(t *testing.T) {
	cases := map[string]string{
		"Go Tutorials":              "Go Tutorials",
		"  Lo-fi / Chill: beats?  ": "Lo-fi Chill beats",
		"..":                        "",
		"../../Windows":             "Windows",
		`C:\Users\me`:               "C Users me",
		"...":                       "",
		"Mix.":                      "Mix",
		".hidden":                   "hidden",
		"CON":                       "CON_",
		"con.txt":                   "con.txt_",
		"LPT1":                      "LPT1_",
		"COMPUTER":                  "COMPUTER",
		"Tab\tthere\nnewline":       "Tab there newline",
		"\x00":                      "",
		"Ethiopian music · ሙዚቃ":     "Ethiopian music · ሙዚቃ",
	}
	for in, want := range cases {
		if got := SafeFolderName(in); got != want {
			t.Errorf("SafeFolderName(%q) = %q, want %q", in, got, want)
		}
	}
	long := SafeFolderName(strings.Repeat("a", 300))
	if len([]rune(long)) != maxFolderName {
		t.Fatalf("long name has %d characters", len([]rune(long)))
	}
}
