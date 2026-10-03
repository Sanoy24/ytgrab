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
	"github.com/Sanoy24/ytgrab/internal/autostart"
	"github.com/Sanoy24/ytgrab/internal/config"
	"github.com/Sanoy24/ytgrab/internal/reveal"
	"github.com/Sanoy24/ytgrab/internal/selfupdate"
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
	if exe, err := os.Executable(); err == nil {
		selfupdate.CleanUp(exe) // what an earlier update left beside this program
	}

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

	actions := tray.Actions{Open: openPage, OpenFolder: func(dir string) {
		if err := reveal.OpenFolder(dir); err != nil {
			fmt.Fprintf(os.Stderr, "could not open %s: %v\n", dir, err)
		}
	}}
	if login := autostart.ForThisProgram(); login.Supported() {
		// Keep "Start with Windows" pointing at this copy after a move or Scoop update.
		if err := login.Refresh(); err != nil {
			fmt.Fprintf(os.Stderr, "could not update the sign-in entry: %v\n", err)
		}
		actions.StartAtLogin, actions.SetStartAtLogin = login.Enabled, login.Set
		actions.StartAtLoginLabel = autostart.Label()
	}
	err := tray.Run(ctx, actions, func(ctx context.Context, hooks tray.Hooks) error {
		serverCfg := cfg
		serverCfg.Ready, serverCfg.Activity = hooks.Ready, hooks.Activity
		return app.Run(ctx, serverCfg, os.Stdout)
	})
	if errors.Is(err, app.ErrRestart) {
		return restartUpdated(cfg, args)
	}
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

// restartUpdated starts the updated program in place of this one, in the background with
// its tray icon; the server, port, and data folder are already released.
func restartUpdated(cfg config.Config, args []string) int {
	cfg.OpenBrowser = false
	if err := startInBackground(cfg, withoutOpen(args)); err != nil {
		fmt.Fprintf(os.Stderr, "YTGrab was updated but could not restart (%v); start it again.\n", err)
		return 1
	}
	fmt.Println("YTGrab was updated and restarted in the background.")
	return 0
}

// withoutOpen drops --open: the page that asked for the update is still open.
func withoutOpen(args []string) []string {
	kept := []string{}
	for _, arg := range args {
		if arg != "--open" && arg != "-open" {
			kept = append(kept, arg)
		}
	}
	return kept
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
