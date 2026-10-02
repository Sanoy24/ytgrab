//go:build linux

package autostart

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxEntry(t *testing.T) {
	dir := t.TempDir()
	configDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { configDir = os.UserConfigDir })

	old := Entry{Exe: "/home/me/.local/share/ytgrab/ytgrab"}
	if old.Enabled() {
		t.Fatal("enabled before Set")
	}
	if err := old.Set(true); err != nil || !old.Enabled() {
		t.Fatalf("Set(true) = %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "autostart", "ytgrab.desktop"))
	if !strings.Contains(string(data), "Exec=\"/home/me/.local/share/ytgrab/ytgrab\"\n") || strings.Contains(string(data), "--open") {
		t.Fatalf("entry = %q", data)
	}
	// CI installs the freedesktop checker; elsewhere this step is skipped.
	if validate, err := exec.LookPath("desktop-file-validate"); err == nil {
		if out, err := exec.Command(validate, filepath.Join(dir, "autostart", "ytgrab.desktop")).CombinedOutput(); err != nil {
			t.Fatalf("desktop-file-validate: %v\n%s", err, out)
		}
	}
	moved := Entry{Exe: `/opt/My "Apps"/$ytgrab`}
	if err := moved.Refresh(); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "autostart", "ytgrab.desktop"))
	if !strings.Contains(string(data), `Exec="/opt/My \\"Apps\\"/\\$ytgrab"`) {
		t.Fatalf("after Refresh = %q", data)
	}
	if err := moved.Set(false); err != nil || moved.Enabled() {
		t.Fatalf("Set(false) = %v", err)
	}
	if err := moved.Set(false); err != nil {
		t.Fatalf("turning off twice = %v", err)
	}
}
