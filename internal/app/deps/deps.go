package deps

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sanoy24/ytgrab/internal/config"
)

// Tool describes one executable needed for YouTube downloads.
type Tool struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Required  bool   `json:"required"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message"`
	// Outdated marks a yt-dlp old enough that YouTube downloads are likely to break.
	Outdated bool `json:"outdated,omitempty"`
}

// offlineMaxAgeDays flags yt-dlp as outdated when the latest release is unknown.
const offlineMaxAgeDays = 90

// ytdlpAgeDays reads yt-dlp's date-based version (2026.08.19, or 2026.08.19.1 for a
// hotfix) and returns its age in days.
func ytdlpAgeDays(version string, now time.Time) (int, bool) {
	parts := strings.Split(version, ".")
	if len(parts) < 3 {
		return 0, false
	}
	released, err := time.Parse("2006.01.02", strings.Join(parts[:3], "."))
	if err != nil {
		return 0, false
	}
	return int(now.Sub(released).Hours() / 24), true
}

// MarkOutdated flags yt-dlp when a newer release exists (latest, if known), or when it
// is very old and the latest release is unknown. yt-dlp's zero-padded date versions
// compare correctly as strings.
func MarkOutdated(report *Report, latest string, now time.Time) {
	for i := range report.Dependencies {
		tool := &report.Dependencies[i]
		if tool.Name != "yt-dlp" || !tool.Available {
			continue
		}
		if latest != "" {
			if _, ok := ytdlpAgeDays(latest, now); ok && tool.Version < latest {
				tool.Outdated = true
				tool.Message = fmt.Sprintf("yt-dlp %s is available. Update when YouTube downloads start failing.", latest)
			}
			continue
		}
		if days, ok := ytdlpAgeDays(tool.Version, now); ok && days > offlineMaxAgeDays {
			tool.Outdated = true
			tool.Message = fmt.Sprintf("This yt-dlp is %d days old. YouTube changes often, so update it.", days)
		}
	}
}

// Report is a point-in-time executable check, not a network download test.
type Report struct {
	Status       string    `json:"status"`
	CheckedAt    time.Time `json:"checked_at"`
	Dependencies []Tool    `json:"dependencies"`
	Note         string    `json:"note"`
}

// Check finds external executables in the configured tools directory, local
// tools directories, then PATH. It never contacts YouTube or downloads files.
func Check(ctx context.Context, cfg config.Config) Report {
	dirs := searchDirs(cfg)
	checks := []func() Tool{
		func() Tool {
			return probe(ctx, dirs, "yt-dlp", true, "--version", "Install yt-dlp and add it to PATH or the tools directory.")
		},
		func() Tool {
			return probe(ctx, dirs, "ffmpeg", true, "-version", "Install ffmpeg and add it to PATH or the tools directory.")
		},
		func() Tool {
			return probe(ctx, dirs, "ffprobe", true, "-version", "Install ffprobe and add it to PATH or the tools directory.")
		},
		func() Tool { return probeRuntime(ctx, dirs) },
	}
	tools := make([]Tool, len(checks))
	var group sync.WaitGroup
	for index, check := range checks {
		group.Add(1)
		go func() {
			defer group.Done()
			tools[index] = check()
		}()
	}
	group.Wait()

	status := "ready"
	for _, tool := range tools {
		if tool.Required && !tool.Available {
			status = "degraded"
			break
		}
	}
	return Report{
		Status:       status,
		CheckedAt:    time.Now().UTC(),
		Dependencies: tools,
		Note:         "Executable checks only. YouTube access and yt-dlp-ejs availability are not verified.",
	}
}

func probeRuntime(ctx context.Context, dirs []string) Tool {
	var failure string
	for _, runtimeName := range []string{"deno", "node"} {
		path, err := findTool(dirs, runtimeName)
		if err != nil {
			continue
		}
		version, err := getVersion(ctx, path, "--version")
		if err != nil {
			failure = fmt.Sprintf("%s was found but could not run: %v", runtimeName, err)
			continue
		}
		if !supportedRuntime(runtimeName, version) {
			failure = fmt.Sprintf("%s %s is too old or its version could not be parsed; update the runtime.", runtimeName, version)
			continue
		}
		message := runtimeName + " is available."
		if runtimeName == "node" {
			message = "Node is available; yt-dlp must be run with --js-runtimes node."
		}
		return Tool{Name: "js-runtime", Required: true, Available: true, Path: path, Version: version, Message: message}
	}
	if failure == "" {
		failure = "Install Deno (recommended) or Node for YouTube JavaScript challenges."
	}
	return Tool{Name: "js-runtime", Required: true, Message: failure}
}

func supportedRuntime(name, version string) bool {
	if name == "deno" {
		version = strings.TrimPrefix(version, "deno ")
	} else {
		version = strings.TrimPrefix(version, "v")
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	if name == "deno" {
		return major > 2 || (major == 2 && minor >= 3)
	}
	return major >= 22
}

func probe(ctx context.Context, dirs []string, name string, required bool, versionFlag string, missingMessage string) Tool {
	path, err := findTool(dirs, name)
	if err != nil {
		return Tool{Name: name, Required: required, Message: missingMessage}
	}
	version, err := getVersion(ctx, path, versionFlag)
	if err != nil {
		return Tool{Name: name, Required: required, Path: path, Message: fmt.Sprintf("Found %s, but its version check failed: %v", name, err)}
	}
	return Tool{Name: name, Required: required, Available: true, Path: path, Version: version, Message: "Available."}
}

// programDir returns the folder of the real program file. When started through a symlink,
// os.Executable may return the link on some systems (macOS), and tools/ lives beside the
// real file.
func programDir(exe string) string {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// ProgramDir is the folder containing the running program, with symlinks resolved.
func ProgramDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return programDir(exe), nil
}

func searchDirs(cfg config.Config) []string {
	var dirs []string
	if cfg.ToolsDir != "" {
		dirs = append(dirs, cfg.ToolsDir)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(programDir(exe), "tools"))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, "tools"))
	}
	// winget links newly installed tools here; this process's PATH may predate them.
	if local := os.Getenv("LOCALAPPDATA"); runtime.GOOS == "windows" && local != "" {
		dirs = append(dirs, filepath.Join(local, "Microsoft", "WinGet", "Links"))
	}
	return dirs
}

func findTool(dirs []string, name string) (string, error) {
	fileNames := []string{name}
	if runtime.GOOS == "windows" {
		fileNames = []string{name + ".exe"}
	}
	for _, dir := range dirs {
		for _, fileName := range fileNames {
			path := filepath.Join(dir, fileName)
			info, err := os.Stat(path)
			if err == nil && info.Mode().IsRegular() {
				return path, nil
			}
		}
	}
	return exec.LookPath(name)
}

// Find resolves an executable using the same search order as health checks.
func Find(cfg config.Config, name string) (string, error) {
	return findTool(searchDirs(cfg), name)
}

// Version runs path --version, using the same cache as the health check.
func Version(ctx context.Context, path string) (string, error) {
	return getVersion(ctx, path, "--version")
}

// versionTimeout allows for the Windows yt-dlp.exe, which unpacks itself on every run
// and takes about three seconds, longer on a first run while antivirus scans it.
const versionTimeout = 15 * time.Second

type versionKey struct {
	path    string
	size    int64
	modTime time.Time
}

// versions caches successful version checks per file, so health checks don't re-run
// slow tools; a replaced or updated file has a new size or modification time.
var versions sync.Map // versionKey -> string

func getVersion(ctx context.Context, path string, flag string) (string, error) {
	var key versionKey
	if info, err := os.Stat(path); err == nil {
		key = versionKey{path: path, size: info.Size(), modTime: info.ModTime()}
		if version, ok := versions.Load(key); ok {
			return version.(string), nil
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	output, err := exec.CommandContext(checkCtx, path, flag).CombinedOutput()
	if err != nil {
		if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("timed out after %v", versionTimeout)
		}
		return "", err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("version command returned no output")
	}
	if key.path != "" {
		versions.Store(key, line)
	}
	return line, nil
}
