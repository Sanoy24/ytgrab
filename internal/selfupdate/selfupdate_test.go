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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func zipped(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = entry.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func tarred(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(gz)
	for name, content := range files {
		if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		_, _ = writer.Write([]byte(content))
	}
	_ = writer.Close()
	_ = gz.Close()
	return buffer.Bytes()
}

// release serves archives and their checksum files like GitHub's download URLs.
func release(t *testing.T, archives map[string][]byte, wrongSum bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for name, data := range archives {
		sum := sha256.Sum256(data)
		text := hex.EncodeToString(sum[:])
		if wrongSum {
			text = hex.EncodeToString(make([]byte, sha256.Size))
		}
		mux.HandleFunc("/download/v1.13.0/"+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) })
		mux.HandleFunc("/download/v1.13.0/"+name+".sha256", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(text + "  " + name + "\n"))
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestArchiveName(t *testing.T) {
	for _, c := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "ytgrab-1.13.0-windows-amd64.zip"},
		{"linux", "arm64", "ytgrab-1.13.0-linux-arm64.tar.gz"},
		{"darwin", "arm64", ""},
		{"windows", "arm64", ""},
	} {
		if got := ArchiveName("1.13.0", c.goos, c.goarch); got != c.want {
			t.Errorf("%s/%s = %q", c.goos, c.goarch, got)
		}
	}
}

func TestFetchChecksAndExtracts(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ytgrab.exe")
	server := release(t, map[string][]byte{
		"ytgrab-1.13.0-windows-amd64.zip":  zipped(t, map[string]string{"ytgrab.exe": "new windows program", "tools/yt-dlp.exe": "tool", "README.md": "docs"}),
		"ytgrab-1.13.0-linux-amd64.tar.gz": tarred(t, map[string]string{"./ytgrab": "new linux program", "./README.md": "docs"}),
	}, false)

	got, err := Fetch(context.Background(), server.Client(), server.URL, "1.13.0", "windows", "amd64", exe)
	if err != nil || got != exe+".new" {
		t.Fatalf("Fetch = %q, %v", got, err)
	}
	if data, _ := os.ReadFile(got); string(data) != "new windows program" {
		t.Fatalf("program = %q", data)
	}
	got, err = Fetch(context.Background(), server.Client(), server.URL, "1.13.0", "linux", "amd64", filepath.Join(dir, "ytgrab"))
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(got); string(data) != "new linux program" {
		t.Fatalf("program = %q", data)
	}
	if _, err := Fetch(context.Background(), server.Client(), server.URL, "1.13.0", "linux", "arm64", exe); err == nil {
		t.Error("a missing archive was accepted")
	}
	if _, err := Fetch(context.Background(), server.Client(), server.URL, "1.13.0", "darwin", "arm64", exe); !errors.Is(err, ErrUnsupported) {
		t.Errorf("macOS = %v", err)
	}
}

func TestFetchRefusesABadChecksum(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ytgrab.exe")
	server := release(t, map[string][]byte{"ytgrab-1.13.0-windows-amd64.zip": zipped(t, map[string]string{"ytgrab.exe": "tampered"})}, true)
	if _, err := Fetch(context.Background(), server.Client(), server.URL, "1.13.0", "windows", "amd64", exe); !errors.Is(err, ErrChecksum) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(exe + ".new"); !os.IsNotExist(err) {
		t.Error("an unverified program was written")
	}
}

func TestFetchNeedsTheProgramAtTheTop(t *testing.T) {
	server := release(t, map[string][]byte{"ytgrab-1.13.0-windows-amd64.zip": zipped(t, map[string]string{"sub/ytgrab.exe": "nested"})}, false)
	if _, err := Fetch(context.Background(), server.Client(), server.URL, "1.13.0", "windows", "amd64", filepath.Join(t.TempDir(), "ytgrab.exe")); err == nil {
		t.Error("a program in a subfolder was used")
	}
}

func TestReplaceAndCleanUp(t *testing.T) {
	for _, goos := range []string{"windows", "linux"} {
		dir := t.TempDir()
		exe := filepath.Join(dir, "ytgrab.exe")
		_ = os.WriteFile(exe, []byte("old"), 0o755)
		_ = os.WriteFile(exe+".new", []byte("new"), 0o755)
		if err := Replace(exe, exe+".new", goos); err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		if data, _ := os.ReadFile(exe); string(data) != "new" {
			t.Errorf("%s: exe = %q", goos, data)
		}
		_, oldErr := os.Stat(exe + ".old")
		if (goos == "windows") != (oldErr == nil) {
			t.Errorf("%s: old copy kept = %v", goos, oldErr == nil)
		}
		CleanUp(exe)
		if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
			t.Errorf("%s: old copy left after CleanUp", goos)
		}
	}
	// A failed swap puts the running program back.
	dir := t.TempDir()
	exe := filepath.Join(dir, "ytgrab.exe")
	_ = os.WriteFile(exe, []byte("old"), 0o755)
	if err := Replace(exe, filepath.Join(dir, "missing.new"), "windows"); err == nil {
		t.Fatal("a missing program was put in place")
	}
	if data, _ := os.ReadFile(exe); string(data) != "old" {
		t.Errorf("exe after a failed swap = %q", data)
	}
}
