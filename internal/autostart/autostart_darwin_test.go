//go:build darwin

package autostart

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoginAgent(t *testing.T) {
	dir := t.TempDir()
	homeDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { homeDir = os.UserHomeDir })
	path := filepath.Join(dir, "Library", "LaunchAgents", agentLabel+".plist")

	entry := Entry{Exe: "/opt/homebrew/Cellar/ytgrab/1.8.0/bin/ytgrab"}
	if entry.Enabled() {
		t.Fatal("enabled before Set")
	}
	if err := entry.Set(true); err != nil || !entry.Enabled() {
		t.Fatalf("Set(true) = %v", err)
	}
	// Apple's own checker accepts the file.
	if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %v\n%s", err, out)
	}
	moved := Entry{Exe: "/Users/me/Apps & Tools/ytgrab"}
	if err := moved.Refresh(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "<string>/Users/me/Apps &amp; Tools/ytgrab</string>") {
		t.Fatalf("after Refresh:\n%s", data)
	}
	if out, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %v\n%s", err, out)
	}
	if err := moved.Set(false); err != nil || moved.Enabled() {
		t.Fatalf("Set(false) = %v", err)
	}
}
