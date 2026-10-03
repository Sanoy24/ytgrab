// Package selfupdate replaces this copy of YTGrab with a newer release: it downloads the
// release archive for this system, checks it against the SHA-256 file published with it
// (as install.sh does), makes sure the new program runs, and swaps the program file. On
// Windows the running program can't be overwritten, only renamed, so it is kept beside
// the new one as ytgrab.exe.old until the next start removes it.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// Limits on what is downloaded; release archives are a few tens of megabytes.
const (
	maxArchive = 200 << 20
	maxProgram = 150 << 20
)

var (
	ErrUnsupported = errors.New("releases don't include a build for this system")
	ErrChecksum    = errors.New("the download doesn't match the release's checksum")
)

// ArchiveName is the release archive for a system, or "" when releases have none for it.
// macOS is left out: its builds are untested, and the install script also keeps an app
// bundle up to date there.
func ArchiveName(version, goos, goarch string) string {
	switch {
	case goos == "windows" && goarch == "amd64":
		return "ytgrab-" + version + "-windows-amd64.zip"
	case goos == "linux" && (goarch == "amd64" || goarch == "arm64"):
		return "ytgrab-" + version + "-linux-" + goarch + ".tar.gz"
	}
	return ""
}

// ProgramName is the program's file name inside a release archive.
func ProgramName(goos string) string {
	if goos == "windows" {
		return "ytgrab.exe"
	}
	return "ytgrab"
}

// Fetch downloads version's archive from releasesURL (like
// https://github.com/Sanoy24/ytgrab/releases), checks it, and writes the program inside
// it next to exe as exe+".new", returning that path.
func Fetch(ctx context.Context, client *http.Client, releasesURL, version, goos, goarch, exe string) (string, error) {
	name := ArchiveName(version, goos, goarch)
	if name == "" {
		return "", ErrUnsupported
	}
	base := strings.TrimSuffix(releasesURL, "/") + "/download/v" + version + "/" + name
	sum, err := get(ctx, client, base+".sha256", 4096)
	if err != nil {
		return "", fmt.Errorf("download the checksum: %w", err)
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", errors.New("the release's checksum file can't be read")
	}
	archive, err := get(ctx, client, base, maxArchive)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	got := sha256.Sum256(archive)
	if !strings.EqualFold(hex.EncodeToString(got[:]), fields[0]) {
		return "", ErrChecksum
	}
	program, err := extract(archive, name, ProgramName(goos))
	if err != nil {
		return "", err
	}
	target := exe + ".new"
	if err := os.WriteFile(target, program, 0o755); err != nil {
		return "", err
	}
	return target, nil
}

func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("the file is larger than expected")
	}
	return data, nil
}

// extract returns the program from the top level of a .zip or .tar.gz release archive.
func extract(archive []byte, name, program string) ([]byte, error) {
	missing := fmt.Errorf("%s has no %s", name, program)
	if strings.HasSuffix(name, ".zip") {
		reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if path.Clean(file.Name) != program || !file.Mode().IsRegular() {
				continue
			}
			content, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer content.Close()
			return readLimited(content)
		}
		return nil, missing
	}
	unzipped, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	reader := tar.NewReader(unzipped)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil, missing
		}
		if err != nil {
			return nil, err
		}
		if path.Clean(header.Name) == program && header.Typeflag == tar.TypeReg {
			return readLimited(reader)
		}
	}
}

func readLimited(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxProgram+1))
	if err == nil && len(data) > maxProgram {
		err = errors.New("the program in the archive is larger than expected")
	}
	return data, err
}

// Verify runs the new program with --version and checks that it reports version, which
// also shows that it runs on this system.
func Verify(ctx context.Context, program, version string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, program, "--version").Output()
	if err != nil {
		return fmt.Errorf("the new version didn't start: %w", err)
	}
	if got := strings.TrimSpace(string(output)); got != version {
		return fmt.Errorf("the new program reports version %q, not %s", got, version)
	}
	return nil
}

// Replace puts the program at program in exe's place. On Windows the running exe is moved
// to exe+".old" first, and put back if the swap fails.
func Replace(exe, program, goos string) error {
	if goos != "windows" {
		return os.Rename(program, exe) // the running program keeps its old file open
	}
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := os.Rename(program, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}

// CleanUp removes what an earlier update left beside exe: the replaced program and any
// download that wasn't put in place. Either may still be in use for a moment; then the
// next start removes it.
func CleanUp(exe string) {
	_ = os.Remove(exe + ".old")
	_ = os.Remove(exe + ".new")
}
