package deps

import (
	"context"
	"github.com/Sanoy24/ytgrab/internal/config"
	"os"
	"path/filepath"
	"runtime"
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

func TestRuntimeArgsNameTheCheckedRuntime(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("YTGRAB_FAKE_TOOL_LOG", filepath.Join(t.TempDir(), "runs.log"))
	t.Setenv("PATH", "") // only the tools folder below
	// Named as the lookup expects: "deno.exe" on Windows, "deno" elsewhere (the test
	// binary itself is "deps.test" there).
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	tools := func(names ...string) config.Config {
		dir := t.TempDir()
		for _, name := range names {
			copyFile(t, self, filepath.Join(dir, name+suffix))
		}
		return config.Config{ToolsDir: dir}
	}

	t.Setenv("YTGRAB_FAKE_VERSION_DENO", "deno 2.9.7")
	t.Setenv("YTGRAB_FAKE_VERSION_NODE", "v24.4.1")
	cfg := tools("deno", "node")
	if got := RuntimeArgs(context.Background(), cfg); len(got) != 2 || got[0] != "--js-runtimes" || got[1] != "deno:"+filepath.Join(cfg.ToolsDir, "deno"+suffix) {
		t.Errorf("current Deno = %v", got)
	}
	// A Deno too old for YouTube's challenges gives way to a current Node, by path.
	t.Setenv("YTGRAB_FAKE_VERSION_DENO", "deno 2.1.0")
	cfg = tools("deno", "node")
	if got := RuntimeArgs(context.Background(), cfg); len(got) != 2 || got[1] != "node:"+filepath.Join(cfg.ToolsDir, "node"+suffix) {
		t.Errorf("old Deno, current Node = %v", got)
	}
	t.Setenv("YTGRAB_FAKE_VERSION_NODE", "v20.0.0")
	if got := RuntimeArgs(context.Background(), tools("deno", "node")); got != nil {
		t.Errorf("nothing supported = %v", got)
	}
}
