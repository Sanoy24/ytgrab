package deps

import "testing"

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
