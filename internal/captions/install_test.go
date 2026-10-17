package captions

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, text := range files {
		w, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(text))
	}
	_ = writer.Close()
	return buffer.Bytes()
}

func sum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func TestUnpackKeepsOnlyWhisperAndItsLibraries(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "whisper-bin-x64.zip")
	_ = os.WriteFile(archive, zipOf(t, map[string]string{
		"Release/whisper-cli.exe":     "cli",
		"Release/ggml.dll":            "lib",
		"Release/test-vad.exe":        "test",
		"Release/../../evil.dll":      "escape attempt",
		"Release/sub/whisper-cli.exe": "cli again",
	}), 0o600)
	out := filepath.Join(dir, "whisper")
	if err := unpack(archive, out); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(out)
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["whisper-cli.exe"] || !names["ggml.dll"] || names["test-vad.exe"] || len(names) != 3 {
		t.Errorf("unpacked %v", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.dll")); !os.IsNotExist(err) {
		t.Error("a file was written outside the folder")
	}
}

func TestInstallChecksEveryDownload(t *testing.T) {
	build := zipOf(t, map[string]string{"Release/whisper-cli.exe": "cli", "Release/ggml.dll": "lib"})
	modelData := []byte("pretend model")
	serveModel := modelData
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/model" {
			_, _ = w.Write(serveModel)
			return
		}
		_, _ = w.Write(build)
	}))
	defer server.Close()
	oldBuilds, oldModel, oldURL, oldModelURL := builds, model, buildURL, modelSource
	t.Cleanup(func() { builds, model, buildURL, modelSource = oldBuilds, oldModel, oldURL, oldModelURL })
	builds = map[string]download{"windows/amd64": {"whisper-bin-x64.zip", sum(build), 1 << 20}}
	model = download{"ggml-base.bin", sum(modelData), int64(len(modelData))}
	buildURL = func(string) string { return server.URL + "/build" }
	modelSource = func() string { return server.URL + "/model" }

	newService := func() *Service {
		s := New(t.TempDir(), func() (string, error) { return "ffmpeg", nil }, func(context.Context, string) (domain.Job, error) { return domain.Job{}, nil }, nil)
		s.goos, s.goarch = "windows", "amd64"
		return s
	}
	wait := func(s *Service) Status {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for s.Status().Installing && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		return s.Status()
	}

	// A model whose checksum doesn't match is refused, and nothing is left behind.
	serveModel = []byte("tampered model")
	bad := newService()
	if err := bad.Install(); err != nil {
		t.Fatal(err)
	}
	if status := wait(bad); status.Installed || status.Error == "" {
		t.Fatalf("tampered install = %+v", status)
	}
	if _, err := os.Stat(bad.modelPath()); !os.IsNotExist(err) {
		t.Error("the tampered model was kept")
	}

	serveModel = modelData
	good := newService()
	if status := good.Status(); !status.Supported || status.Installed {
		t.Fatalf("before installing = %+v", status)
	}
	_ = good.Install()
	if status := wait(good); !status.Installed || status.Error != "" || status.Progress != 1 {
		t.Fatalf("after installing = %+v", status)
	}
}

func TestMacUsesHomebrewsWhisper(t *testing.T) {
	s := New(t.TempDir(), nil, nil, nil)
	s.goos, s.goarch = "darwin", "arm64"
	s.lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	old := homebrewWhisper
	homebrewWhisper = nil // even on a Mac that has it
	t.Cleanup(func() { homebrewWhisper = old })
	if status := s.Status(); !status.Supported || status.HowTo == "" {
		t.Errorf("mac without whisper = %+v", status)
	}
	s.goos = "freebsd"
	if status := s.Status(); status.Supported {
		t.Errorf("unsupported system = %+v", status)
	}
}
