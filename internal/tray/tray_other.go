//go:build !windows

package tray

import "context"

// Run calls serve. Only Windows has a tray icon so far; elsewhere YTGrab runs in the
// terminal.
func Run(ctx context.Context, _ Actions, serve Serve) error {
	return serve(ctx, Hooks{Ready: func(string, func() string) {}})
}
