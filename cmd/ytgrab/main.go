package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"ytgrab/internal/app"
	"ytgrab/internal/config"
)

func main() {
	os.Exit(run())
}

// version is set at release build time with -ldflags "-X main.version=...".
var version = "dev"

func run() int {
	open := flag.Bool("open", false, "open the app in the default browser after starting")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return 0
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return 1
	}
	cfg.Version = version
	cfg.OpenBrowser = *open

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		return 1
	}
	return 0
}
