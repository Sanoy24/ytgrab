package web

import (
	"embed"
	"io/fs"
)

//go:embed static/*
var assets embed.FS

// Static returns the browser assets rooted at web/static.
func Static() fs.FS {
	static, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err) // The directory is guaranteed by the embed directive.
	}
	return static
}
