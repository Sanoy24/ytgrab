//go:build windows

package tray

import (
	"context"
	_ "embed"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/Sanoy24/ytgrab/internal/activity"
)

//go:embed ytgrab.ico
var icon []byte

// Run calls serve and, once the server is listening, shows a tray icon with Open and
// Quit until serve returns. Quit cancels serve's context, which shuts the server down
// like Ctrl+C. Run must be called from the main goroutine.
func Run(ctx context.Context, actions Actions, serve Serve) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	listening := make(chan string, 1)

	// Queue reports can arrive before the icon exists; keep the latest for its tooltip.
	var mu sync.Mutex
	showing := false
	tooltip := "YTGrab is running"
	onActivity := func(summary activity.Summary) {
		mu.Lock()
		tooltip = summary.Tooltip()
		ready := showing
		mu.Unlock()
		if !ready {
			return
		}
		systray.SetTooltip(summary.Tooltip())
		for _, n := range notifications(summary) {
			notify(n.Title, n.Text)
		}
	}
	hooks := Hooks{Ready: func(url string) { listening <- url }, Activity: onActivity}
	go func() { done <- serve(ctx, hooks) }()

	var url string
	select {
	case err := <-done:
		return err // never listened: no icon
	case url = <-listening:
	}

	result := make(chan error, 1)
	systray.Run(func() {
		systray.SetIcon(icon)
		mu.Lock()
		showing = true
		systray.SetTooltip(tooltip)
		mu.Unlock()
		systray.SetOnTapped(func() { actions.Open(url) })
		open := systray.AddMenuItem("Open YTGrab", "Show YTGrab in your browser")
		systray.AddSeparator()
		var login *systray.MenuItem
		var loginClicked <-chan struct{}
		var resync <-chan time.Time
		var ticker *time.Ticker
		if actions.StartAtLogin != nil && actions.SetStartAtLogin != nil {
			login = systray.AddMenuItemCheckbox("Start with Windows", "Start YTGrab in the tray when you sign in", actions.StartAtLogin())
			loginClicked = login.ClickedCh
			// The page can change the setting too; keep the tick in step with it.
			ticker = time.NewTicker(5 * time.Second)
			resync = ticker.C
			systray.AddSeparator()
		}
		quit := systray.AddMenuItem("Quit YTGrab", "Stop YTGrab; unfinished downloads resume next time")
		showLogin := func(on bool) {
			if on {
				login.Check()
			} else {
				login.Uncheck()
			}
		}
		go func() {
			if ticker != nil {
				defer ticker.Stop()
			}
			for {
				select {
				case <-open.ClickedCh:
					actions.Open(url)
				case <-loginClicked:
					on := !actions.StartAtLogin()
					if err := actions.SetStartAtLogin(on); err == nil {
						showLogin(on)
					}
				case <-resync:
					showLogin(actions.StartAtLogin())
				case <-quit.ClickedCh:
					cancel()
				case <-ctx.Done():
					return
				}
			}
		}()
		// Only end the tray loop once it is running, so Quit is never lost.
		go func() {
			result <- <-done
			systray.Quit()
		}()
	}, nil)
	return <-result
}
