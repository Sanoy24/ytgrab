package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/api"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.8.0", "1.7.0", true},
		{"1.10.0", "1.9.3", true},
		{"2.0.0", "1.99.99", true},
		{"1.7.0", "1.7.0", false},
		{"1.6.9", "1.7.0", false},
		{"v1.8.0", "1.7.0", true},
		{"", "1.7.0", false},
		{"1.8.0", "dev", false},
		{"1.8.0-rc.1", "1.7.0", false},
	}
	for _, c := range cases {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestUpdateCommand(t *testing.T) {
	cases := []struct {
		exe, goos, want string
	}{
		{`C:\Users\me\scoop\apps\ytgrab\current\ytgrab.exe`, "windows", "scoop update ytgrab"},
		{`D:\Scoop\apps\ytgrab\1.7.0\ytgrab.exe`, "windows", "scoop update ytgrab"},
		{"/opt/homebrew/Cellar/ytgrab/1.7.0/bin/ytgrab", "darwin", "brew update && brew upgrade ytgrab"},
		{"/home/linuxbrew/.linuxbrew/Cellar/ytgrab/1.7.0/bin/ytgrab", "linux", "brew update && brew upgrade ytgrab"},
		{"/home/me/.local/share/ytgrab/ytgrab", "linux", "curl -fsSL https://raw.githubusercontent.com/Sanoy24/ytgrab/main/install.sh | sh"},
		{"/home/me/go/bin/ytgrab", "linux", "go install github.com/Sanoy24/ytgrab/cmd/ytgrab@latest"},
		{`C:\Apps\YTGrab\ytgrab.exe`, "windows", ""},
	}
	for _, c := range cases {
		if got := updateCommand(c.exe, c.goos); got != c.want {
			t.Errorf("updateCommand(%q) = %q, want %q", c.exe, got, c.want)
		}
	}
}

func TestDevBuildsDontLookForUpdates(t *testing.T) {
	updates := &ytgrabUpdates{version: "dev", latest: &latestRelease{}}
	if info := updates.YTGrabVersion(); info.UpdateAvailable || info.Latest != "" {
		t.Fatalf("dev build info = %+v", info)
	}
	updates = &ytgrabUpdates{version: "1.7.0", latest: &latestRelease{}, command: "scoop update ytgrab"}
	updates.latest.set("1.8.0")
	if info := updates.YTGrabVersion(); !info.UpdateAvailable || info.UpdateCommand != "scoop update ytgrab" || info.Latest != "1.8.0" {
		t.Fatalf("info = %+v", info)
	}
}

func TestCanUpdateItself(t *testing.T) {
	cases := []struct {
		exe, goos, goarch string
		want              bool
	}{
		{`C:\Apps\YTGrab\ytgrab.exe`, "windows", "amd64", true},
		{"/home/me/.local/share/ytgrab/ytgrab", "linux", "amd64", true},
		{"/home/me/apps/ytgrab/ytgrab", "linux", "arm64", true},
		{`C:\Users\me\scoop\apps\ytgrab\current\ytgrab.exe`, "windows", "amd64", false},
		{"/home/linuxbrew/.linuxbrew/Cellar/ytgrab/1.7.0/bin/ytgrab", "linux", "amd64", false},
		{"/home/me/go/bin/ytgrab", "linux", "amd64", false},
		{"/Users/me/.local/share/ytgrab/ytgrab", "darwin", "arm64", false},
		{`C:\Apps\YTGrab\ytgrab.exe`, "windows", "arm64", false},
	}
	for _, c := range cases {
		if got := canUpdateItself(c.exe, c.goos, c.goarch); got != c.want {
			t.Errorf("canUpdateItself(%q, %s/%s) = %v", c.exe, c.goos, c.goarch, got)
		}
	}
}

func TestUpdateYTGrabRefuses(t *testing.T) {
	known := func(version string) *latestRelease {
		return &latestRelease{version: version, checked: time.Now()}
	}
	restart := func() { t.Error("restarted") }
	cases := map[string]struct {
		updates *ytgrabUpdates
		want    error
	}{
		"up to date":        {&ytgrabUpdates{version: "1.13.0", latest: known("1.13.0"), selfUpdate: true, restart: restart}, api.ErrNoYTGrabUpdate},
		"package manager":   {&ytgrabUpdates{version: "1.12.0", latest: known("1.13.0"), selfUpdate: false, restart: restart}, api.ErrNoYTGrabUpdate},
		"development build": {&ytgrabUpdates{version: "dev", latest: known("1.13.0"), selfUpdate: true, restart: restart}, api.ErrNoYTGrabUpdate},
		"downloads running": {&ytgrabUpdates{version: "1.12.0", latest: known("1.13.0"), selfUpdate: true, restart: restart, running: func() int { return 1 }}, api.ErrYTGrabDownloadsRunning},
		"no way to restart": {&ytgrabUpdates{version: "1.12.0", latest: known("1.13.0"), selfUpdate: true}, api.ErrNoYTGrabUpdate},
	}
	for name, c := range cases {
		if _, err := c.updates.UpdateYTGrab(context.Background()); !errors.Is(err, c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
