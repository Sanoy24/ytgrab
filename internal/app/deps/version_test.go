package deps

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// When YTGRAB_FAKE_TOOL_LOG is set, this test binary acts as a tool that records each
// run and prints a version, so version checks can be observed.
func TestMain(m *testing.M) {
	if log := os.Getenv("YTGRAB_FAKE_TOOL_LOG"); log != "" {
		f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			_, _ = f.WriteString("run\n")
			_ = f.Close()
		}
		os.Stdout.WriteString("9.9.9\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// The Windows yt-dlp.exe unpacks itself on every run and takes about three seconds.
func TestVersionTimeoutAllowsSlowTools(t *testing.T) {
	if versionTimeout < 10*time.Second {
		t.Fatalf("versionTimeout = %v; slow self-extracting tools would be reported missing", versionTimeout)
	}
}

func TestVersionIsCachedUntilTheFileChanges(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool"+filepath.Ext(self))
	copyFile(t, self, tool)
	log := filepath.Join(dir, "runs.log")
	t.Setenv("YTGRAB_FAKE_TOOL_LOG", log)

	for range 2 {
		if version, err := getVersion(context.Background(), tool, "--version"); err != nil || version != "9.9.9" {
			t.Fatalf("getVersion = %q, %v", version, err)
		}
	}
	if runs := countRuns(t, log); runs != 1 {
		t.Fatalf("tool ran %d times, want 1 (cached)", runs)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(tool, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := getVersion(context.Background(), tool, "--version"); err != nil {
		t.Fatal(err)
	}
	if runs := countRuns(t, log); runs != 2 {
		t.Fatalf("tool ran %d times after it changed, want 2", runs)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
}

func countRuns(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "run\n")
}
