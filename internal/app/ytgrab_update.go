package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sanoy24/ytgrab/internal/api"
	"github.com/Sanoy24/ytgrab/internal/selfupdate"
	"github.com/Sanoy24/ytgrab/internal/setup"
)

// ErrRestart is returned by Run after an update was installed: start the program again.
var ErrRestart = errors.New("restart to finish updating YTGrab")

// ytgrabUpdates tells the page when a newer YTGrab is out, and how this copy is updated:
// by itself, or with a command for the package manager that installed it.
type ytgrabUpdates struct {
	version    string
	latest     *latestRelease
	command    string
	exe        string
	selfUpdate bool
	running    func() int // downloads in progress
	restart    func()     // ends Run with ErrRestart
	busy       sync.Mutex
}

func newYTGrabUpdates(version string) *ytgrabUpdates {
	exe, _ := os.Executable()
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved // Homebrew links bin/ytgrab into its Cellar
	}
	return &ytgrabUpdates{version: version, latest: newLatestYTGrab(), command: updateCommand(exe, runtime.GOOS), exe: exe,
		selfUpdate: canUpdateItself(exe, runtime.GOOS, runtime.GOARCH)}
}

// canUpdateItself reports copies that YTGrab may replace: unpacked from a release archive or
// put in place by install.sh, on a system with release builds. Package managers update
// their own copies.
func canUpdateItself(exe, goos, goarch string) bool {
	if selfupdate.ArchiveName("0.0.0", goos, goarch) == "" {
		return false
	}
	path := strings.ToLower(strings.ReplaceAll(exe, `\`, "/"))
	return updateCommand(exe, goos) == "" || strings.Contains(path, "/.local/share/ytgrab/")
}

// UpdateYTGrab installs the latest release over this copy, then restarts YTGrab.
func (u *ytgrabUpdates) UpdateYTGrab(ctx context.Context) (api.YTGrabUpdate, error) {
	latest := u.latest.Get()
	if !u.selfUpdate || u.restart == nil || u.version == "" || u.version == "dev" || !newer(latest, u.version) {
		return api.YTGrabUpdate{}, api.ErrNoYTGrabUpdate
	}
	if u.running != nil && u.running() > 0 {
		return api.YTGrabUpdate{}, api.ErrYTGrabDownloadsRunning
	}
	if !u.busy.TryLock() {
		return api.YTGrabUpdate{}, api.ErrUpdateBusy
	}
	defer u.busy.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	program, err := selfupdate.Fetch(ctx, http.DefaultClient, setup.YTGrabReleasesURL, latest, runtime.GOOS, runtime.GOARCH, u.exe)
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return api.YTGrabUpdate{}, errors.New("YTGrab can't write to its folder; update it the way you installed it")
		}
		return api.YTGrabUpdate{}, err
	}
	if err := selfupdate.Verify(ctx, program, latest); err != nil {
		_ = os.Remove(program)
		return api.YTGrabUpdate{}, err
	}
	if err := selfupdate.Replace(u.exe, program, runtime.GOOS); err != nil {
		_ = os.Remove(program)
		return api.YTGrabUpdate{}, err
	}
	// Answer first, then restart: the page waits for the new version to come up.
	time.AfterFunc(500*time.Millisecond, u.restart)
	return api.YTGrabUpdate{Version: latest, Restarting: true}, nil
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
		info.CanUpdate = u.selfUpdate && u.restart != nil
	}
	return info
}

// updateCommand returns the command that updates a copy of YTGrab installed at exe, or
// "" when it was unpacked from a release archive and is updated by downloading it again.
func updateCommand(exe, goos string) string {
	path := strings.ToLower(strings.ReplaceAll(exe, `\`, "/")) // same result on every system
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
