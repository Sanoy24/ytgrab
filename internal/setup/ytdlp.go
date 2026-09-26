package setup

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ytdlpReleaseURL serves the latest official yt-dlp release assets.
const ytdlpReleaseURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download"

const maxYtdlpSize = 200 << 20

// ytdlpAsset returns the official standalone yt-dlp build for a platform.
func ytdlpAsset(goos, goarch string) (string, error) {
	switch {
	case goos == "windows":
		return "yt-dlp.exe", nil
	case goos == "darwin":
		return "yt-dlp_macos", nil
	case goos == "linux" && goarch == "arm64":
		return "yt-dlp_linux_aarch64", nil
	case goos == "linux":
		return "yt-dlp_linux", nil
	default:
		return "", fmt.Errorf("no official yt-dlp build for %s/%s; install it with your package manager", goos, goarch)
	}
}

// downloadYtdlp fetches the platform's yt-dlp and the release's SHA2-256SUMS, and
// installs the binary into dir only when its SHA-256 matches.
func downloadYtdlp(ctx context.Context, client *http.Client, baseURL, goos, goarch, dir string) (string, error) {
	asset, err := ytdlpAsset(goos, goarch)
	if err != nil {
		return "", err
	}
	want, err := expectedChecksum(ctx, client, baseURL+"/SHA2-256SUMS", asset)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := "yt-dlp"
	if goos == "windows" {
		name = "yt-dlp.exe"
	}
	target := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, ".yt-dlp-download-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename

	body, err := get(ctx, client, baseURL+"/"+asset)
	if err != nil {
		tmp.Close()
		return "", err
	}
	defer body.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(body, maxYtdlpSize+1))
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("download %s: %w", asset, err)
	}
	if n > maxYtdlpSize {
		return "", fmt.Errorf("download %s: file is unexpectedly large", asset)
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want {
		return "", fmt.Errorf("download %s: checksum %s does not match the official %s; nothing was installed", asset, got, want)
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		return "", fmt.Errorf("install %s: %w (is yt-dlp running?)", target, err)
	}
	return target, nil
}

func expectedChecksum(ctx context.Context, client *http.Client, url, asset string) (string, error) {
	body, err := get(ctx, client, url)
	if err != nil {
		return "", err
	}
	defer body.Close()
	scanner := bufio.NewScanner(io.LimitReader(body, 1<<20))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset && len(fields[0]) == 64 {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("the official checksum list has no entry for %s", asset)
}

func get(ctx context.Context, client *http.Client, url string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ytgrab-setup")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("download %s: %s", url, response.Status)
	}
	return response.Body, nil
}
