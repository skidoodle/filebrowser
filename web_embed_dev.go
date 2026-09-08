//go:build dev

package main

import (
	"io/fs"
)

// webFS reports that no frontend bundle is embedded in dev builds.
func webFS() (fs.FS, error) {
	return nil, errNoEmbed
}
