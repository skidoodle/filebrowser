package fsutil

import (
	"os"
	"path"
	"strings"
)

const (
	maxPathLen    = 4096
	maxSegmentLen = 255
)

// Clean validates and normalizes a slash-separated relative path.
// Empty paths map to "." (the storage root). Absolute paths, backslashes,
// ".." segments, NUL bytes and over-long paths are rejected with
// os.ErrInvalid so callers can map them to 400s.
func Clean(p string) (string, error) {
	if p == "" {
		return ".", nil
	}
	if strings.ContainsAny(p, "\x00\\") {
		return "", os.ErrInvalid
	}
	if strings.HasPrefix(p, "/") {
		return "", os.ErrInvalid
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == ".." {
			return "", os.ErrInvalid
		}
		if len(seg) > maxSegmentLen {
			return "", os.ErrInvalid
		}
	}
	c := path.Clean(p)
	if c == "." || c == "/" {
		if c == "/" {
			return "", os.ErrInvalid
		}
		return ".", nil
	}
	if strings.HasPrefix(c, "../") || c == ".." {
		return "", os.ErrInvalid
	}
	if len(c) > maxPathLen {
		return "", os.ErrInvalid
	}
	return c, nil
}
