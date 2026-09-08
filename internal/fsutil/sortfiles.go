package fsutil

import (
	"slices"

	"github.com/skidoodle/filebrowser/internal/storage"
)

// SortFileInfos sorts items in place: directories always come first,
// then entries ordered by the requested key (natural order for names).
// Descending order applies within both groups.
func SortFileInfos(items []storage.FileInfo, opt storage.SortOptions) {
	slices.SortFunc(items, func(a, b storage.FileInfo) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		r := 0
		switch opt.By {
		case "size":
			switch {
			case a.Size < b.Size:
				r = -1
			case a.Size > b.Size:
				r = 1
			}
		case "modified":
			r = a.ModTime.Compare(b.ModTime)
		default:
			r = NaturalCompare(a.Name, b.Name)
		}
		if r == 0 {
			r = NaturalCompare(a.Name, b.Name)
		}
		if !opt.Asc {
			r = -r
		}
		return r
	})
}
