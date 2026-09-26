package main

import (
	"context"
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

func run() int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		return 1
	}
	return 0
}
