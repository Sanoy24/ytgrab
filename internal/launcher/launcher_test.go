package launcher

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// as runs a test as if on another system, in a home folder of its own.
func as(t *testing.T, system string) string {
	t.Helper()
	home := t.TempDir()
	oldGOOS, oldHome, oldRefresh := goos, homeDir, refresh
	goos, homeDir, refresh = system, func() (string, error) { return home, nil }, func(string) {}
	t.Setenv("XDG_DATA_HOME", "")
	t.Cleanup(func() { goos, homeDir, refresh = oldGOOS, oldHome, oldRefresh })
	return home
}

func TestMacApp(t *testing.T) {
	home := as(t, "darwin")
	app, err := Install("/opt/homebrew/opt/ytgrab/bin/ytgrab", "1.18.0")
	if err != nil {
		t.Fatal(err)
	}
	if app != filepath.Join(home, "Applications", "YTGrab.app") {
		t.Fatalf("app = %s", app)
	}
	plist, _ := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	for _, want := range []string{"<string>YTGrab</string>", "<key>CFBundleIconFile</key>\n\t<string>ytgrab</string>", "<string>1.18.0</string>", "<key>LSUIElement</key>\n\t<true/>"} {
		if !strings.Contains(string(plist), want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}
	script := filepath.Join(app, "Contents", "MacOS", "YTGrab")
	if data, _ := os.ReadFile(script); string(data) != "#!/bin/sh\nexec \"/opt/homebrew/opt/ytgrab/bin/ytgrab\" --open\n" {
		t.Errorf("launcher = %q", data)
	}
	if info, err := os.Stat(script); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0) {
		t.Errorf("launcher not executable: %v %v", info, err)
	}
	if data, _ := os.ReadFile(filepath.Join(app, "Contents", "Resources", "ytgrab.icns")); !bytes.Equal(data, icns) {
		t.Error("icon missing")
	}

	// Running it again (after an update) just rewrites it.
	if _, err := Install("/opt/homebrew/opt/ytgrab/bin/ytgrab", "1.19.0"); err != nil {
		t.Fatal(err)
	}
	if removed, err := Remove(); err != nil || removed != app {
		t.Fatalf("Remove = %q, %v", removed, err)
	}
	if _, err := os.Stat(app); !os.IsNotExist(err) {
		t.Errorf("app still there: %v", err)
	}
}

func TestLinuxMenuEntry(t *testing.T) {
	home := as(t, "linux")
	entry, err := Install("/home/me/.local/share/ytgrab/ytgrab", "1.18.0")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(entry)
	icon := filepath.Join(home, ".local", "share", "icons", "hicolor", "512x512", "apps", "ytgrab.png")
	for _, want := range []string{"Name=YTGrab\n", "Exec=\"/home/me/.local/share/ytgrab/ytgrab\" --open\n", "Icon=" + icon + "\n", "Terminal=false\n"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("ytgrab.desktop lacks %q:\n%s", want, data)
		}
	}
	if got, _ := os.ReadFile(icon); !bytes.Equal(got, png) {
		t.Error("icon missing")
	}
	if _, err := Remove(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{entry, icon} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s still there", path)
		}
	}
}

func TestRefusesPathsALauncherCantHold(t *testing.T) {
	as(t, "darwin")
	if _, err := Install(`/Users/me/"odd"/ytgrab`, "1.18.0"); err == nil {
		t.Error("accepted a quote in the path")
	}
	as(t, "windows")
	if _, err := Install(`C:\ytgrab\ytgrab.exe`, "1.18.0"); err != ErrUnsupported {
		t.Errorf("windows = %v", err)
	}
}

func TestStablePathSurvivesHomebrewUpgrades(t *testing.T) {
	prefix := t.TempDir()
	versioned := filepath.Join(prefix, "Cellar", "ytgrab", "1.18.0", "bin", "ytgrab")
	if got := StablePath(versioned); got != versioned {
		t.Errorf("without an opt link = %s", got)
	}
	opt := filepath.Join(prefix, "opt", "ytgrab", "bin", "ytgrab")
	if err := os.MkdirAll(filepath.Dir(opt), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opt, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := StablePath(versioned); got != opt {
		t.Errorf("StablePath = %s, want %s", got, opt)
	}
	if other := "/usr/local/bin/ytgrab"; StablePath(other) != other {
		t.Error("changed a path outside Homebrew")
	}
}

func TestIconsMatchTheReleaseIcons(t *testing.T) {
	for name, embedded := range map[string][]byte{"ytgrab.icns": icns, "ytgrab.png": png} {
		release, err := os.ReadFile(filepath.Join("..", "..", "packaging", "icon", name))
		if err != nil || !bytes.Equal(release, embedded) {
			t.Errorf("internal/launcher/%s differs from packaging/icon/%s; copy it over", name, name)
		}
	}
}
