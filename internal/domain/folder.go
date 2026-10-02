package domain

import (
	"strings"
	"unicode"
)

// maxFolderName keeps folder names well inside path-length limits.
const maxFolderName = 80

// SafeFolderName turns a title (like a playlist's) into a single folder name that is
// valid on Windows, macOS, and Linux, or "" when nothing usable is left. It can never
// name a parent folder or contain a path separator.
func SafeFolderName(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case unicode.IsControl(r), strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	name := strings.Join(strings.Fields(b.String()), " ")
	if runes := []rune(name); len(runes) > maxFolderName {
		name = string(runes[:maxFolderName])
	}
	// Windows drops trailing dots and spaces; leading dots hide folders elsewhere.
	name = strings.Trim(name, ". ")
	if name == "" {
		return ""
	}
	if reservedOnWindows(name) {
		name += "_"
	}
	return name
}

// reservedOnWindows reports device names Windows won't use as a folder, with or without
// an extension (CON, con.txt, LPT1, ...).
func reservedOnWindows(name string) bool {
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	base = strings.TrimRight(base, " ")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return true
	}
	return false
}
