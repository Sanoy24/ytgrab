//go:build windows

package autostart

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestSetEnabledAndRefresh(t *testing.T) {
	parent := fmt.Sprintf(`Software\YTGrab-test-%d`, os.Getpid())
	runKey = parent + `\Run`
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, runKey)
		_ = registry.DeleteKey(registry.CURRENT_USER, parent)
	})

	old := Entry{Exe: `C:\Old\ytgrab.exe`}
	if old.Enabled() {
		t.Fatal("enabled before Set")
	}
	if err := old.Set(false); err != nil {
		t.Fatalf("turning off when absent: %v", err)
	}
	if err := old.Set(true); err != nil || !old.Enabled() {
		t.Fatalf("Set(true) = %v, enabled %v", err, old.Enabled())
	}
	if value, _ := old.current(); value != `"C:\Old\ytgrab.exe"` {
		t.Fatalf("command = %q", value)
	}

	moved := Entry{Exe: `C:\New Folder\ytgrab.exe`}
	if err := moved.Refresh(); err != nil {
		t.Fatal(err)
	}
	if value, _ := moved.current(); value != `"C:\New Folder\ytgrab.exe"` {
		t.Fatalf("after Refresh, command = %q", value)
	}

	if err := moved.Set(false); err != nil || moved.Enabled() {
		t.Fatalf("Set(false) = %v, enabled %v", err, moved.Enabled())
	}
	if err := moved.Refresh(); err != nil || moved.Enabled() {
		t.Fatalf("Refresh turned it back on: %v", err)
	}
	if err := (Entry{}).Set(true); err != ErrUnsupported {
		t.Fatalf("empty entry Set = %v", err)
	}
}
