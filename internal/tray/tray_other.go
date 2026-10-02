//go:build !windows && !linux && !(darwin && cgo)

package tray

import "context"

// Run calls serve. There is no menu-bar icon on this system yet; YTGrab runs in the
// terminal.
func Run(ctx context.Context, _ Actions, serve Serve) error {
	return serve(ctx, Hooks{Ready: func(string, func() string) {}})
}
