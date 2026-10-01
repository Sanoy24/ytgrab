package deps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSupportedJavaScriptRuntimeVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{"deno", "deno 2.3.0", true},
		{"deno", "deno 2.2.9", false},
		{"node", "v24.4.1", true},
		{"node", "v20.19.0", false},
		{"node", "unknown", false},
	}
	for _, tc := range tests {
		if got := supportedRuntime(tc.name, tc.version); got != tc.want {
			t.Errorf("supportedRuntime(%q, %q) = %v, want %v", tc.name, tc.version, got, tc.want)
		}
	}
}

func TestProgramDirFollowsSymlinks(t *testing.T) {
	real := filepath.Join(t.TempDir(), "ytgrab")
	if err := os.WriteFile(real, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "ytgrab")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	if got, want := programDir(link), filepath.Dir(real); got != want {
		resolvedWant, _ := filepath.EvalSymlinks(want)
		if got != resolvedWant {
			t.Fatalf("programDir(%q) = %q, want %q", link, got, want)
		}
	}
}
