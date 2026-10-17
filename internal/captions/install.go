package captions

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// whisperVersion is the whisper.cpp release YTGrab installs; its builds' checksums were
// taken from the release and are checked before anything is unpacked.
const whisperVersion = "v1.9.2"

type download struct {
	name   string
	sha256 string
	size   int64 // the exact size, for the model; a limit for builds
}

var builds = map[string]download{
	"windows/amd64": {"whisper-bin-x64.zip", "49dcc16de826f20bd53d44f947a1ae49dfa81f86cad67a64d80820cb192d674a", 64 << 20},
	"linux/amd64":   {"whisper-bin-ubuntu-x64.tar.gz", "46811a3ecf584307480a220b9ef5ff81b7b22dc41577cbc274ce3afc61f753b1", 64 << 20},
	"linux/arm64":   {"whisper-bin-ubuntu-arm64.tar.gz", "7e26fa6a36d9174d5c0bf033ccbc026c3b5e569e2ee787058241346ef5392719", 64 << 20},
}

// model is whisper's multilingual "base" model: a good balance of accuracy and speed on an
// ordinary computer (an hour of speech in about ten minutes on a recent 8-core laptop).
var model = download{"ggml-base.bin", "60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe", 147951465}

const modelURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin"

// Replaced in tests.
var (
	buildURL = func(name string) string {
		return "https://github.com/ggml-org/whisper.cpp/releases/download/" + whisperVersion + "/" + name
	}
	modelSource = func() string { return modelURL }
)

// Install downloads whisper.cpp (except on macOS, where Homebrew provides it) and the
// speech model in the background. Status reports progress.
func (s *Service) Install() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.installing {
		return nil
	}
	build, buildable := builds[s.goos+"/"+s.goarch]
	if !buildable && s.goos != "darwin" {
		return errors.New("Local captions aren't available for this system yet.")
	}
	s.installing, s.progress, s.installErr = true, 0, ""
	go func() {
		err := s.install(context.Background(), build, buildable)
		s.mu.Lock()
		defer s.mu.Unlock()
		s.installing = false
		if err != nil {
			s.installErr = err.Error()
		}
	}()
	return nil
}

func (s *Service) install(ctx context.Context, build download, buildable bool) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	if buildable {
		if _, err := s.whisper(); err != nil {
			archive := filepath.Join(s.dir, build.name)
			if err := s.fetch(ctx, buildURL(build.name), archive, build, 0, 0.05); err != nil {
				return err
			}
			defer os.Remove(archive)
			if err := unpack(archive, filepath.Join(s.dir, "whisper")); err != nil {
				return fmt.Errorf("unpack whisper.cpp: %w", err)
			}
		}
	}
	if !s.modelReady() {
		if err := s.fetch(ctx, modelSource(), s.modelPath(), model, 0.05, 1); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.progress = 1
	s.mu.Unlock()
	return nil
}

// fetch downloads url to target only if its SHA-256 matches, reporting progress between
// from and to.
func (s *Service) fetch(ctx context.Context, url, target string, want download, from, to float64) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "ytgrab-setup")
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", want.name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", want.name, response.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	hash := sha256.New()
	total := response.ContentLength
	if total <= 0 {
		total = want.size
	}
	var done int64
	buffer := make([]byte, 256<<10)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			done += int64(n)
			if done > want.size+1<<20 {
				tmp.Close()
				return fmt.Errorf("download %s: the file is unexpectedly large", want.name)
			}
			if _, err := tmp.Write(buffer[:n]); err != nil {
				tmp.Close()
				return err
			}
			hash.Write(buffer[:n])
			s.mu.Lock()
			s.progress = from + (to-from)*min(1, float64(done)/float64(total))
			s.mu.Unlock()
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmp.Close()
			return fmt.Errorf("download %s: %w", want.name, readErr)
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != want.sha256 {
		return fmt.Errorf("download %s: the checksum doesn't match the official one, so nothing was installed", want.name)
	}
	return os.Rename(tmp.Name(), target)
}

// unpack puts whisper-cli and its libraries flat into dir: only those files, by base name,
// so nothing in the archive can write elsewhere.
func unpack(archive, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(archive, ".zip") {
		reader, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer reader.Close()
		for _, file := range reader.File {
			name := path.Base(file.Name)
			if file.FileInfo().IsDir() || !wanted(name) {
				continue
			}
			src, err := file.Open()
			if err != nil {
				return err
			}
			err = writeFile(filepath.Join(dir, name), src, 0o755)
			src.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Base(header.Name)
		switch header.Typeflag {
		case tar.TypeReg:
			if wanted(name) {
				if err := writeFile(filepath.Join(dir, name), reader, 0o755); err != nil {
					return err
				}
			}
		case tar.TypeSymlink:
			// libggml.so → libggml.so.0 → libggml.so.0.18.1: links within the folder only.
			if wanted(name) && !strings.ContainsAny(header.Linkname, `/\`) {
				_ = os.Remove(filepath.Join(dir, name))
				if err := os.Symlink(header.Linkname, filepath.Join(dir, name)); err != nil {
					return err
				}
			}
		}
	}
}

// wanted keeps the command-line program and the libraries it loads.
func wanted(name string) bool {
	lower := strings.ToLower(name)
	return lower == "whisper-cli" || lower == "whisper-cli.exe" ||
		strings.HasSuffix(lower, ".dll") || strings.Contains(lower, ".so")
}

func writeFile(target string, src io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(src, 256<<20)); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
