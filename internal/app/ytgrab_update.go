package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/setup"
)

// ytgrabUpdates tells the page when a newer YTGrab is out, and how this copy is updated.
type ytgrabUpdates struct {
	version string
	latest  *latestRelease
	command string
}

func newYTGrabUpdates(version string) *ytgrabUpdates {
	exe, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved // Homebrew links bin/ytgrab into its Cellar
	}
	return &ytgrabUpdates{version: version, latest: newLatestYTGrab(), command: updateCommand(exe, runtime.GOOS)}
}

func (u *ytgrabUpdates) YTGrabVersion() api.VersionInfo {
	info := api.VersionInfo{Version: u.version, ReleaseURL: setup.YTGrabReleasesURL + "/latest"}
	if u.version == "" || u.version == "dev" {
		return info // development builds don't look for updates
	}
	info.Latest = u.latest.Get()
	if newer(info.Latest, u.version) {
		info.UpdateAvailable = true
		info.UpdateCommand = u.command
	}
	return info
}

// updateCommand returns the command that updates a copy of YTGrab installed at exe, or
// "" when it was unpacked from a release archive and is updated by downloading it again.
func updateCommand(exe, goos string) string {
	path := strings.ToLower(filepath.ToSlash(exe))
	switch {
	case strings.Contains(path, "/scoop/apps/ytgrab/"):
		return "scoop update ytgrab"
	case strings.Contains(path, "/cellar/ytgrab/"):
		return "brew upgrade ytgrab"
	case strings.Contains(path, "/.local/share/ytgrab/") && goos != "windows":
		return "curl -fsSL https://raw.githubusercontent.com/Sanoy24/ytgrab/main/install.sh | sh"
	case strings.Contains(path, "/go/bin/"):
		return "go install github.com/Sanoy24/ytgrab/cmd/ytgrab@latest"
	default:
		return ""
	}
}

// newer reports whether version a (like "1.8.0") is later than b. Anything that isn't
// three numbers is never newer.
func newer(a, b string) bool {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var parts [3]int
	fields := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

// ytgrabUpdatesFor starts the first release check right away, like the yt-dlp one, so
// the first page load can already show a notice.
func ytgrabUpdatesFor(version string) *ytgrabUpdates {
	updates := newYTGrabUpdates(version)
	if version != "" && version != "dev" {
		updates.latest.Get()
	}
	return updates
}
