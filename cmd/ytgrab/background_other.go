//go:build !windows

package main

import "github.com/Sanoy24/ytgrab/internal/config"

// Outside Windows YTGrab always runs in the terminal it was started from.
func inBackground() bool                              { return false }
func startedFromShortcut() bool                       { return false }
func startInBackground(config.Config, []string) error { return nil }
func showError(string)                                {}
