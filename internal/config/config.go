package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const defaultListenAddress = "127.0.0.1:8787"

// Config contains the settings needed to start the local application.
type Config struct {
	ListenAddress   string
	ToolsDir        string
	ShutdownTimeout time.Duration
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

	cfg := Config{
		ListenAddress:   address,
		ToolsDir:        toolsDir,
		ShutdownTimeout: 5 * time.Second,
	}
	return cfg, cfg.Validate()
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
	return nil
}
