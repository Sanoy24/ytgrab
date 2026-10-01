package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/setup"
	sqlitestore "github.com/Sanoy24/ytgrab/internal/store/sqlite"
)

// DoctorChecks reports app-level readiness for "ytgrab doctor": the data folder, the
// chosen download folder, and whether the port is free.
func DoctorChecks(cfg config.Config) []setup.Line {
	lines := []setup.Line{writableLine("data", cfg.DataDir, true)}
	lines = append(lines, downloadFolderLine(cfg))
	url := "http://" + cfg.ListenAddress + "/"
	if listener, err := net.Listen("tcp", cfg.ListenAddress); err == nil {
		_ = listener.Close()
		lines = append(lines, setup.Line{Status: "ok", Name: "port", Detail: cfg.ListenAddress + " is free"})
	} else {
		lines = append(lines, setup.Line{Status: "warn", Name: "port", Detail: cfg.ListenAddress + " is in use. YTGrab may already be running: open " + url})
	}
	return lines
}

func downloadFolderLine(cfg config.Config) setup.Line {
	dir, chosen := cfg.DownloadsDir, cfg.DownloadsDirSet
	dbPath := filepath.Join(cfg.DataDir, "jobs.db")
	if _, err := os.Stat(dbPath); err == nil {
		if store, err := sqlitestore.Open(context.Background(), dbPath); err == nil {
			if saved, found, err := store.GetSetting(context.Background(), "downloads_dir"); err == nil && found {
				dir, chosen = saved, true
			}
			store.Close()
		}
	}
	if !chosen {
		return setup.Line{Status: "info", Name: "downloads", Detail: "No folder chosen yet; the page asks for one before the first download."}
	}
	return writableLine("downloads", dir, false)
}

func writableLine(name, dir string, create bool) setup.Line {
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return setup.Line{Status: "missing", Name: name, Detail: fmt.Sprintf("%s cannot be created: %v", dir, err), Fail: true}
		}
	}
	probe, err := os.CreateTemp(dir, ".ytgrab-doctor-*")
	if err != nil {
		detail := dir + " is not writable"
		if os.IsNotExist(err) || strings.Contains(err.Error(), "cannot find") {
			detail = dir + " does not exist; choose another folder in the page"
		}
		return setup.Line{Status: "missing", Name: name, Detail: detail, Fail: true}
	}
	probe.Close()
	os.Remove(probe.Name())
	return setup.Line{Status: "ok", Name: name, Detail: dir}
}

// SetupToolsDir is where "ytgrab setup" installs yt-dlp: YTGRAB_TOOLS_DIR, else tools/
// beside the program (or in the working directory for "go run" builds).
func SetupToolsDir(cfg config.Config) string {
	if cfg.ToolsDir != "" {
		return cfg.ToolsDir
	}
	if dir, err := deps.ProgramDir(); err == nil && !strings.Contains(filepath.ToSlash(dir), "/go-build") {
		return filepath.Join(dir, "tools")
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "tools")
}
