// Package setup implements "ytgrab doctor" (check what the app needs) and "ytgrab setup"
// (install what is missing, asking before each step).
package setup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"

	"ytgrab/internal/app/deps"
)

// Env holds everything setup touches, so tests can replace it.
type Env struct {
	GOOS, GOARCH string
	Check        func(context.Context) deps.Report
	LookPath     func(string) (string, error)
	Run          func(ctx context.Context, name string, args ...string) error
	HTTP         *http.Client
	ReleaseURL   string
	ToolsDir     string // where yt-dlp is installed
	Extra        func() []Line
	In           io.Reader
	Out          io.Writer
	Yes          bool // answer yes to every question

	answers *bufio.Reader
}

type Options struct {
	UpdateYtdlp bool // download the latest yt-dlp even if one is installed
}

// Line is one non-tool doctor result, such as the data folder or the port.
type Line struct {
	Status string // ok, missing, warn, info
	Name   string
	Detail string
	Fail   bool // counts as a problem
}

// SystemEnv returns an Env for the real machine.
func SystemEnv(check func(context.Context) deps.Report, toolsDir string, in io.Reader, out io.Writer) Env {
	return Env{
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Check: check, LookPath: exec.LookPath, Run: runInteractive,
		HTTP: http.DefaultClient, ReleaseURL: ytdlpReleaseURL, ToolsDir: toolsDir,
		In: in, Out: out,
	}
}

func runInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

var versionNumber = regexp.MustCompile(`\d+(?:\.\d+)+`)

// shortVersion turns "ffmpeg version 8.0.1-full_build-www.gyan.dev Copyright…" into "8.0.1".
func shortVersion(version string) string {
	if number := versionNumber.FindString(version); number != "" {
		return number
	}
	return version
}

// Doctor prints the state of every requirement and reports whether all required ones pass.
func Doctor(ctx context.Context, env Env) bool {
	report := env.Check(ctx)
	problems := 0
	fmt.Fprintln(env.Out, "Tools")
	for _, tool := range report.Dependencies {
		status, detail := "ok", shortVersion(tool.Version)
		if !tool.Available {
			status, detail = "missing", tool.Message
			if tool.Required {
				problems++
			}
		} else if tool.Path != "" {
			detail += "  " + tool.Path
		}
		fmt.Fprintf(env.Out, "  %-9s %-11s %s\n", "["+status+"]", tool.Name, detail)
	}
	if env.Extra != nil {
		fmt.Fprintln(env.Out, "App")
		for _, line := range env.Extra() {
			if line.Fail {
				problems++
			}
			fmt.Fprintf(env.Out, "  %-9s %-11s %s\n", "["+line.Status+"]", line.Name, line.Detail)
		}
	}
	fmt.Fprintln(env.Out)
	switch problems {
	case 0:
		fmt.Fprintln(env.Out, "Everything YTGrab needs is ready.")
	case 1:
		fmt.Fprintln(env.Out, `1 problem found. Run "ytgrab setup" to fix it.`)
	default:
		fmt.Fprintf(env.Out, "%d problems found. Run \"ytgrab setup\" to fix them.\n", problems)
	}
	return problems == 0
}

type packageInstall struct {
	label        string
	winget, brew string
	manual       string // shown when nothing can be installed automatically
	missing      func(map[string]deps.Tool) bool
}

var packages = []packageInstall{
	{
		label: "FFmpeg (merges video and audio, converts MP3)", winget: "Gyan.FFmpeg", brew: "ffmpeg",
		manual: "sudo apt install ffmpeg   (Fedora: sudo dnf install ffmpeg, Arch: sudo pacman -S ffmpeg)",
		missing: func(tools map[string]deps.Tool) bool {
			return !tools["ffmpeg"].Available || !tools["ffprobe"].Available
		},
	},
	{
		label: "Deno (runs YouTube's JavaScript checks)", winget: "DenoLand.Deno", brew: "deno",
		manual:  "curl -fsSL https://deno.land/install.sh | sh",
		missing: func(tools map[string]deps.Tool) bool { return !tools["js-runtime"].Available },
	},
}

// Setup installs missing requirements, asking before each one, then runs Doctor again.
func Setup(ctx context.Context, env Env, options Options) error {
	env.answers = bufio.NewReader(env.In)
	tools := map[string]deps.Tool{}
	for _, tool := range env.Check(ctx).Dependencies {
		tools[tool.Name] = tool
	}
	changed := false

	if !tools["yt-dlp"].Available || options.UpdateYtdlp {
		if env.ask(fmt.Sprintf("Download the latest yt-dlp from its official GitHub release into %s?", env.ToolsDir)) {
			fmt.Fprintln(env.Out, "  Downloading yt-dlp and checking its checksum…")
			path, err := downloadYtdlp(ctx, env.HTTP, env.ReleaseURL, env.GOOS, env.GOARCH, env.ToolsDir)
			if err != nil {
				fmt.Fprintf(env.Out, "  Could not install yt-dlp: %v\n", err)
			} else {
				fmt.Fprintf(env.Out, "  Installed %s (checksum verified).\n", path)
				changed = true
			}
		}
	}

	for _, pkg := range packages {
		if !pkg.missing(tools) {
			continue
		}
		name, args, manual := env.installCommand(pkg)
		if name == "" {
			fmt.Fprintf(env.Out, "%s is missing. Install it with:\n  %s\n", pkg.label, manual)
			continue
		}
		command := name + " " + strings.Join(args, " ")
		if !env.ask(fmt.Sprintf("Install %s with %s?", pkg.label, name)) {
			fmt.Fprintf(env.Out, "  Skipped. To install it later, run:\n  %s\n", command)
			continue
		}
		fmt.Fprintf(env.Out, "  Running: %s\n", command)
		if err := env.Run(ctx, name, args...); err != nil {
			fmt.Fprintf(env.Out, "  The install did not finish (%v). Try running the command yourself:\n  %s\n", err, command)
			continue
		}
		changed = true
	}

	if changed {
		fmt.Fprintln(env.Out, "\nChecking again…")
	}
	fmt.Fprintln(env.Out)
	if !Doctor(ctx, env) {
		return errors.New("some requirements are still missing")
	}
	return nil
}

// installCommand returns the package-manager command for pkg, or "" and a manual hint.
func (env Env) installCommand(pkg packageInstall) (string, []string, string) {
	switch env.GOOS {
	case "windows":
		if _, err := env.LookPath("winget"); err == nil {
			return "winget", []string{"install", "--id", pkg.winget, "-e", "--accept-source-agreements", "--accept-package-agreements"}, ""
		}
		return "", nil, "winget install --id " + pkg.winget + "   (winget comes with App Installer from the Microsoft Store)"
	case "darwin":
		if _, err := env.LookPath("brew"); err == nil {
			return "brew", []string{"install", pkg.brew}, ""
		}
		return "", nil, "brew install " + pkg.brew + "   (install Homebrew from https://brew.sh first)"
	default:
		// Linux installs need sudo and a distribution-specific package manager.
		return "", nil, pkg.manual
	}
}

// ask prints question and reads a yes/no answer; Enter means yes.
func (env *Env) ask(question string) bool {
	fmt.Fprintf(env.Out, "%s [Y/n] ", question)
	if env.Yes {
		fmt.Fprintln(env.Out, "yes")
		return true
	}
	line, err := env.answers.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(env.Out)
		return false // no input available: don't install anything unasked
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "" || answer == "y" || answer == "yes"
}
