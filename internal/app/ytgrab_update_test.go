package app

import "testing"

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
		{`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\Sanoy24.YTGrab_Microsoft.Winget.Source_8wekyb3d8bbwe\ytgrab.exe`, "windows", "winget upgrade Sanoy24.YTGrab"},
		{"/opt/homebrew/Cellar/ytgrab/1.7.0/bin/ytgrab", "darwin", "brew upgrade ytgrab"},
		{"/home/linuxbrew/.linuxbrew/Cellar/ytgrab/1.7.0/bin/ytgrab", "linux", "brew upgrade ytgrab"},
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
