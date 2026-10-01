package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func fakeRelease(t *testing.T, asset string, body []byte, sums string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/SHA2-256SUMS":
			_, _ = w.Write([]byte(sums))
		case "/" + asset:
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestDownloadYtdlpVerifiesChecksum(t *testing.T) {
	body := []byte("fake yt-dlp binary")
	sum := sha256.Sum256(body)
	sums := hex.EncodeToString(sum[:]) + "  yt-dlp.exe\n0000  yt-dlp_linux\n"
	server := fakeRelease(t, "yt-dlp.exe", body, sums)
	dir := t.TempDir()

	path, err := downloadYtdlp(context.Background(), server.Client(), server.URL, "windows", "amd64", dir)
	if err != nil || path != filepath.Join(dir, "yt-dlp.exe") {
		t.Fatalf("downloadYtdlp = %q, %v", path, err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(body) {
		t.Fatalf("saved %q", got)
	}
}

func TestDownloadYtdlpRejectsMismatch(t *testing.T) {
	server := fakeRelease(t, "yt-dlp_linux", []byte("tampered"), "0000000000000000000000000000000000000000000000000000000000000000  yt-dlp_linux\n")
	dir := t.TempDir()
	if _, err := downloadYtdlp(context.Background(), server.Client(), server.URL, "linux", "amd64", dir); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("mismatched download left files: %v", entries)
	}
}

func TestYtdlpAssetNames(t *testing.T) {
	for _, test := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "yt-dlp.exe"},
		{"darwin", "arm64", "yt-dlp_macos"},
		{"linux", "amd64", "yt-dlp_linux"},
		{"linux", "arm64", "yt-dlp_linux_aarch64"},
	} {
		if got, _ := ytdlpAsset(test.goos, test.goarch); got != test.want {
			t.Errorf("ytdlpAsset(%s/%s) = %q, want %q", test.goos, test.goarch, got, test.want)
		}
	}
}

func TestLatestYtdlpVersionFollowsTheReleaseRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			http.Redirect(w, r, "/tag/2026.09.30", http.StatusFound)
		case "/tag/2026.09.30":
			_, _ = w.Write([]byte("release page"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	version, err := LatestYtdlpVersion(context.Background(), server.Client(), server.URL)
	if err != nil || version != "2026.09.30" {
		t.Fatalf("LatestYtdlpVersion = %q, %v", version, err)
	}
}
