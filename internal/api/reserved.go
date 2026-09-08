package api

import (
	"net/http"
	"strings"

	"github.com/skidoodle/filebrowser/internal/fsutil"
)

// reservedRoots are first path segments claimed by the SPA's client-side
// routes. Root-level entries with these names could never be browsed: the
// router reads /view, /new, /settings, /login and /setup as application
// pages, never as storage paths. Deeper segments are unaffected
// ("docs/view" is a perfectly good folder). The frontend mirrors this list
// in frontend/src/lib/routes.ts for inline validation.
var reservedRoots = map[string]struct{}{
	"view":     {},
	"new":      {},
	"settings": {},
	"login":    {},
	"setup":    {},
}

// rejectReservedRoot answers 400 when a write target's first segment is
// reserved by a client-side route, covering both explicit directory
// creation and implicit parents of file writes. It reports whether the
// request was rejected. Paths fsutil.Clean rejects are left to the storage
// adapter, which maps them to the same class of error.
func rejectReservedRoot(w http.ResponseWriter, path string) bool {
	cleaned, err := fsutil.Clean(path)
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(cleaned, "/")
	if _, reserved := reservedRoots[first]; !reserved {
		return false
	}
	apiError(w, http.StatusBadRequest, "this name is reserved by the application")
	return true
}
