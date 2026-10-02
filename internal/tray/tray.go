// Package tray shows YTGrab's notification-area icon while the server runs.
package tray

import "context"

// Serve runs the server until ctx ends. It calls ready with the page URL once the
// server is listening, and never calls it if the server fails to start.
type Serve func(ctx context.Context, ready func(url string)) error

// Actions are what the tray menu does.
type Actions struct {
	Open func(url string) // show the page in the browser
}
