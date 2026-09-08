//go:build !dev

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend/dist
var webDist embed.FS

// webFS returns the embedded SPA assets rooted at the dist directory.
func webFS() (fs.FS, error) {
	return fs.Sub(webDist, "frontend/dist")
}
