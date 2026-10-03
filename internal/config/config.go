package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Sanoy24/ytgrab/internal/activity"
)

const defaultListenAddress = "127.0.0.1:8787"

// Config contains the settings needed to start the local application.
type Config struct {
	ListenAddress string
	ToolsDir      string
	DataDir       string
	DownloadsDir  string
	// DownloadsDirSet reports that YTGRAB_DOWNLOAD_DIR was given, which counts as the
	// user's choice of output folder.
	DownloadsDirSet bool
	ShutdownTimeout time.Duration
	// CookiesBrowser is the saved browser-sign-in choice, filled in per download.
	CookiesBrowser string
	// SubtitlesMode ("off", "embed", "file") and SubtitlesLang are filled in per download.
	SubtitlesMode string
	SubtitlesLang string
	// SpeedLimitKBps caps each download in kB/s (0: no limit); filled in per download.
	SpeedLimitKBps int
	// FileNames is the naming style (see settings.FileNameStyles); filled in per download.
	FileNames string
	// SponsorBlock is "off", "mark", or "remove"; filled in per download.
	SponsorBlock string
	// Version and OpenBrowser come from the command line, not the environment.
	Version     string
	OpenBrowser bool
	// Ready, if set, is called once the server is listening, with the page URL and a way
	// to read the current download folder.
	Ready func(url string, downloadsDir func() string)
	// Activity, if set, receives queue summaries for the tray.
	Activity func(activity.Summary)
}

// Load reads local configuration from the environment and validates it.
func Load() (Config, error) {
	address := os.Getenv("YTGRAB_LISTEN_ADDR")
	if address == "" {
		address = defaultListenAddress
	}

	toolsDir := os.Getenv("YTGRAB_TOOLS_DIR")
	if toolsDir != "" {
		absolute, err := filepath.Abs(toolsDir)
		if err != nil {
			return Config{}, fmt.Errorf("YTGRAB_TOOLS_DIR: %w", err)
		}
		toolsDir = absolute
	}
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, fmt.Errorf("find user config directory: %w", err)
	}
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("find user home directory: %w", err)
	}
	dataDir, err := directory("YTGRAB_DATA_DIR", filepath.Join(userConfigDir, "ytgrab"))
	if err != nil {
		return Config{}, err
	}
	downloadsDir, err := directory("YTGRAB_DOWNLOAD_DIR", filepath.Join(userHomeDir, "Downloads", "ytgrab"))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddress:   address,
		ToolsDir:        toolsDir,
		DataDir:         dataDir,
		DownloadsDir:    downloadsDir,
		DownloadsDirSet: os.Getenv("YTGRAB_DOWNLOAD_DIR") != "",
		ShutdownTimeout: 5 * time.Second,
	}
	return cfg, cfg.Validate()
}

func directory(variable string, fallback string) (string, error) {
	value := os.Getenv(variable)
	if value == "" {
		value = fallback
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%s: %w", variable, err)
	}
	return absolute, nil
}

// Validate prevents accidental exposure of the single-user HTTP server.
func (cfg Config) Validate() error {
	host, portText, err := net.SplitHostPort(cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("YTGRAB_LISTEN_ADDR must be a host:port address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("YTGRAB_LISTEN_ADDR must use a loopback IP address (127.0.0.1 or ::1)")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("YTGRAB_LISTEN_ADDR must have a port from 1 to 65535")
	}
	if cfg.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown timeout must be positive")
	}
	if cfg.DataDir == "" || cfg.DownloadsDir == "" {
		return fmt.Errorf("data and download directories must be configured")
	}
	return nil
}
