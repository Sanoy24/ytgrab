// Package tray shows YTGrab's notification-area icon while the server runs.
package tray

import (
	"context"
	"fmt"
	"strings"

	"github.com/Sanoy24/ytgrab/internal/activity"
)

// Hooks are how the server reports to the tray.
type Hooks struct {
	Ready    func(url string, downloadsDir func() string) // the server is listening
	Activity func(activity.Summary)                       // the queue changed; nil where there is no tray
}

// Serve runs the server until ctx ends. It calls hooks.Ready once the server is
// listening, and never calls it if the server fails to start.
type Serve func(ctx context.Context, hooks Hooks) error

// Actions are what the tray menu does.
type Actions struct {
	Open       func(url string) // show the page in the browser
	OpenFolder func(dir string) // show the download folder
	// StartAtLogin and SetStartAtLogin back the start-at-login item, labelled
	// StartAtLoginLabel; leave them nil to hide it.
	StartAtLogin      func() bool
	SetStartAtLogin   func(bool) error
	StartAtLoginLabel string
}

type note struct {
	Title, Text string
}

// notifications turns ended downloads into at most two notifications: one for those that
// finished and one for those that failed.
func notifications(summary activity.Summary) []note {
	var notes []note
	switch n := len(summary.Finished); {
	case n == 1:
		notes = append(notes, note{"Download finished", summary.Finished[0]})
	case n > 1:
		notes = append(notes, note{fmt.Sprintf("%d downloads finished", n), titles(summary.Finished)})
	}
	switch n := len(summary.Failed); {
	case n == 1:
		failure := summary.Failed[0]
		text := failure.Title
		if failure.Message != "" {
			text += "\n" + failure.Message
		}
		notes = append(notes, note{"Download failed", text})
	case n > 1:
		names := make([]string, n)
		for i, failure := range summary.Failed {
			names[i] = failure.Title
		}
		notes = append(notes, note{fmt.Sprintf("%d downloads failed", n), titles(names) + "\nOpen YTGrab to retry them."})
	}
	return notes
}

func titles(names []string) string {
	if len(names) > 3 {
		return strings.Join(names[:3], "\n") + fmt.Sprintf("\nand %d more", len(names)-3)
	}
	return strings.Join(names, "\n")
}
