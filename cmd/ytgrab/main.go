package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Sanoy24/ytgrab/internal/app"
	"github.com/Sanoy24/ytgrab/internal/app/deps"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/setup"
	"github.com/Sanoy24/ytgrab/internal/tray"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// version is set at release build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage:
  ytgrab [--open]              start the app (http://127.0.0.1:8787/)
  ytgrab doctor                check the tools and folders YTGrab needs
  ytgrab setup [--yes] [--update-ytdlp]
                               install what is missing, asking before each step
  ytgrab --version             print the version
`

func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "doctor":
			return doctor(args[1:])
		case "setup":
			return runSetup(args[1:])
		case "help", "-h", "--help", "-help":
			fmt.Print(usage)
			return 0
		}
	}
	flags := flag.NewFlagSet("ytgrab", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(flags.Output(), usage) }
	open := flags.Bool("open", false, "open the app in the default browser after starting")
	showVersion := flags.Bool("version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version)
		return 0
	}
	cfg, ok := loadConfig()
	if !ok {
		return 1
	}
	cfg.OpenBrowser = *open

	// Started from a shortcut on Windows: carry on without a console window, with the
	// tray icon as the way to open or quit YTGrab.
	if startedFromShortcut() {
		if err := startInBackground(cfg, args); err == nil {
			return 0
		}
		// Otherwise keep running in this window.
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := tray.Run(ctx, tray.Actions{Open: openPage}, func(ctx context.Context, ready func(string)) error {
		serverCfg := cfg
		serverCfg.Ready = ready
		return app.Run(ctx, serverCfg, os.Stdout)
	})
	if err != nil {
		var running *app.AlreadyRunningError
		if errors.As(err, &running) {
			// Starting YTGrab again (for example double-clicking the start script twice)
			// just brings up the copy that is already running.
			fmt.Printf("YTGrab is already running at %s\n", running.URL)
			if *open || inBackground() {
				openPage(running.URL)
			}
			return 0
		}
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		if inBackground() {
			showError(fmt.Sprintf("YTGrab could not start:\n\n%v", err))
		}
		return 1
	}
	return 0
}

func openPage(url string) {
	if err := app.OpenBrowser(url); err != nil {
		fmt.Printf("Open %s in your browser.\n", url)
	}
}

func loadConfig() (config.Config, bool) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return cfg, false
	}
	cfg.Version = version
	return cfg, true
}

func toolEnv(cfg config.Config) setup.Env {
	check := func(ctx context.Context) deps.Report {
		report := deps.Check(ctx, cfg)
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		latest, _ := setup.LatestYtdlpVersion(lookupCtx, http.DefaultClient, "") // "" offline
		deps.MarkOutdated(&report, latest, time.Now())
		return report
	}
	env := setup.SystemEnv(check, app.SetupToolsDir(cfg), os.Stdin, os.Stdout)
	env.Extra = func() []setup.Line { return app.DoctorChecks(cfg) }
	return env
}

func doctor(args []string) int {
	flags := flag.NewFlagSet("ytgrab doctor", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg, ok := loadConfig()
	if !ok {
		return 1
	}
	fmt.Printf("YTGrab doctor (version %s)\n\n", version)
	if !setup.Doctor(context.Background(), toolEnv(cfg)) {
		return 1
	}
	return 0
}

func runSetup(args []string) int {
	flags := flag.NewFlagSet("ytgrab setup", flag.ContinueOnError)
	yes := flags.Bool("yes", false, "answer yes to every question")
	update := flags.Bool("update-ytdlp", false, "download the latest yt-dlp even if one is installed")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg, ok := loadConfig()
	if !ok {
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("YTGrab setup (version %s)\n\n", version)
	env := toolEnv(cfg)
	env.Yes = *yes
	if err := setup.Setup(ctx, env, setup.Options{UpdateYtdlp: *update}); err != nil {
		return 1
	}
	return 0
}
