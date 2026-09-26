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

	"ytgrab/internal/config"
)

// Tool describes one executable needed for YouTube downloads.
type Tool struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Required  bool   `json:"required"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message"`
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

func searchDirs(cfg config.Config) []string {
	var dirs []string
	if cfg.ToolsDir != "" {
		dirs = append(dirs, cfg.ToolsDir)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "tools"))
	}
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Join(cwd, "tools"))
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

func getVersion(ctx context.Context, path string, flag string) (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(checkCtx, path, flag).CombinedOutput()
	if err != nil {
		if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("timed out after 3 seconds")
		}
		return "", err
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return "", fmt.Errorf("version command returned no output")
	}
	return line, nil
}
