//go:build windows

package tray

import (
	"context"
	_ "embed"

	"fyne.io/systray"
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
	go func() { done <- serve(ctx, func(url string) { listening <- url }) }()

	var url string
	select {
	case err := <-done:
		return err // never listened: no icon
	case url = <-listening:
	}

	result := make(chan error, 1)
	systray.Run(func() {
		systray.SetIcon(icon)
		systray.SetTooltip("YTGrab is running")
		systray.SetOnTapped(func() { actions.Open(url) })
		open := systray.AddMenuItem("Open YTGrab", "Show YTGrab in your browser")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit YTGrab", "Stop YTGrab; unfinished downloads resume next time")
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					actions.Open(url)
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
