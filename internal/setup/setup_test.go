package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
)

func report(available map[string]bool) deps.Report {
	var tools []deps.Tool
	for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "js-runtime"} {
		tools = append(tools, deps.Tool{Name: name, Required: true, Available: available[name], Version: "1.0", Message: "Install " + name + "."})
	}
	status := "ready"
	for _, tool := range tools {
		if !tool.Available {
			status = "degraded"
		}
	}
	return deps.Report{Status: status, Dependencies: tools}
}

type recorder struct{ commands []string }

func (r *recorder) run(_ context.Context, name string, args ...string) error {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	return nil
}

func TestDoctorReportsMissingTools(t *testing.T) {
	var out bytes.Buffer
	env := Env{Out: &out, Check: func(context.Context) deps.Report {
		return report(map[string]bool{"yt-dlp": true, "ffprobe": true, "js-runtime": true})
	}}
	if Doctor(context.Background(), env) {
		t.Fatal("doctor passed with ffmpeg missing")
	}
	text := out.String()
	for _, want := range []string{"[ok]      yt-dlp", "[missing] ffmpeg", "Install ffmpeg.", "ytgrab setup"} {
		if !strings.Contains(text, want) {
			t.Errorf("doctor output missing %q:\n%s", want, text)
		}
	}
}

func TestSetupOnWindowsAsksBeforeEachInstall(t *testing.T) {
	body := []byte("yt-dlp binary")
	sum := sha256.Sum256(body)
	server := fakeRelease(t, "yt-dlp.exe", body, hex.EncodeToString(sum[:])+"  yt-dlp.exe\n")
	tools := t.TempDir()
	checks := 0
	runner := &recorder{}
	var out bytes.Buffer
	env := Env{
		GOOS: "windows", GOARCH: "amd64",
		Check: func(context.Context) deps.Report {
			checks++
			if checks == 1 {
				return report(nil)
			}
			return report(map[string]bool{"yt-dlp": true, "ffmpeg": true, "ffprobe": true})
		},
		LookPath:   func(name string) (string, error) { return `C:\winget\` + name + ".exe", nil },
		Run:        runner.run,
		HTTP:       server.Client(),
		ReleaseURL: server.URL,
		ToolsDir:   tools,
		In:         strings.NewReader("y\n\nn\n"), // yt-dlp: yes; ffmpeg: default yes; Deno: no
		Out:        &out,
	}
	err := Setup(context.Background(), env, Options{})
	if err == nil {
		t.Fatal("setup reported success while Deno is still missing")
	}
	if _, statErr := os.Stat(filepath.Join(tools, "yt-dlp.exe")); statErr != nil {
		t.Fatalf("yt-dlp not installed: %v\n%s", statErr, out.String())
	}
	if len(runner.commands) != 1 || !strings.Contains(runner.commands[0], "install --id Gyan.FFmpeg -e") {
		t.Fatalf("commands = %v", runner.commands)
	}
	if !strings.Contains(out.String(), "winget install --id DenoLand.Deno") {
		t.Errorf("declined install should print the manual command:\n%s", out.String())
	}
}

func TestSetupOnLinuxPrintsPackageCommands(t *testing.T) {
	runner := &recorder{}
	var out bytes.Buffer
	env := Env{
		GOOS: "linux", GOARCH: "amd64",
		Check:    func(context.Context) deps.Report { return report(map[string]bool{"yt-dlp": true, "js-runtime": true}) },
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Run:      runner.run,
		In:       strings.NewReader(""),
		Out:      &out,
		Yes:      true,
	}
	_ = Setup(context.Background(), env, Options{})
	if len(runner.commands) != 0 {
		t.Fatalf("setup ran %v on Linux; it should only print commands", runner.commands)
	}
	if !strings.Contains(out.String(), "sudo apt install ffmpeg") {
		t.Errorf("missing package hint:\n%s", out.String())
	}
}

func TestDoctorShowsAnUpdateWithoutFailing(t *testing.T) {
	var out bytes.Buffer
	env := Env{Out: &out, Check: func(context.Context) deps.Report {
		r := report(map[string]bool{"yt-dlp": true, "ffmpeg": true, "ffprobe": true, "js-runtime": true})
		r.Dependencies[0].Version = "2026.08.19"
		r.Dependencies[0].Outdated = true
		r.Dependencies[0].Message = "yt-dlp 2026.09.30 is available."
		return r
	}}
	if !Doctor(context.Background(), env) {
		t.Fatalf("an available update should not fail doctor:\n%s", out.String())
	}
	if text := out.String(); !strings.Contains(text, "[update]  yt-dlp") || !strings.Contains(text, "2026.09.30 is available") || !strings.Contains(text, "--update-ytdlp") {
		t.Fatalf("doctor output:\n%s", text)
	}
}
