package api

import (
	"context"
	"crypto/md5"  //nolint:gosec // md5/sha1 are optional integrity checksums for downloads, not crypto
	"crypto/sha1" //nolint:gosec // see above
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/skidoodle/filebrowser/internal/authz"
	"github.com/skidoodle/filebrowser/internal/storage"
)

// handleList returns the contents of a directory.
// Query: path, sort (name|size|modified), order (asc|desc).
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sortBy := "name"
	if v := q.Get("sort"); v == "size" || v == "modified" {
		sortBy = v
	}
	asc := q.Get("order") != "desc"

	if !s.canRead(w, r, q.Get("path")) {
		return
	}
	listing, err := s.store.List(r.Context(), q.Get("path"), storage.SortOptions{By: sortBy, Asc: asc})
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.filterListing(r, listing))
}

// filterListing removes entries hidden inside other users' private folders
// and re-counts the listing so nothing leaks through totals.
func (s *Server) filterListing(r *http.Request, l storage.Listing) storage.Listing {
	if s.cfg.Insecure || s.authz == nil || len(l.Items) == 0 {
		// Insecure mode has no permission engine, but the private flag is
		// still annotated (everyone acts as admin) so folders can be
		// toggled back to public.
		if s.cfg.Insecure && s.authz != nil && len(l.Items) > 0 {
			return rebuildListing(l, nil, s.annotatePrivate(authz.Actor{Admin: true}))
		}
		return l
	}
	actor := s.readerActor(r)
	paths := make([]string, len(l.Items))
	for i, item := range l.Items {
		paths[i] = item.Path
	}
	kept := s.authz.FilterPaths(actor, paths)
	// Annotate even when nothing was filtered: the owner must still see the
	// private flag on their own folders to be able to clear it.
	return rebuildListing(l, kept, s.annotatePrivate(actor))
}

// rebuildListing keeps only the listed paths and recounts the listing.
// An empty kept list keeps everything.
func rebuildListing(l storage.Listing, kept []string, annotate func(storage.FileInfo) storage.FileInfo) storage.Listing {
	keptSet := make(map[string]struct{}, len(kept))
	for _, p := range kept {
		keptSet[p] = struct{}{}
	}
	items := l.Items[:0]
	l.NumDirs, l.NumFiles = 0, 0
	for _, item := range l.Items {
		if _, ok := keptSet[item.Path]; !ok {
			continue
		}
		item = annotate(item)
		if item.IsDir {
			l.NumDirs++
		} else {
			l.NumFiles++
		}
		items = append(items, item)
	}
	l.Items = items
	l.Total = len(items)
	return l
}

// annotatePrivate marks directories the viewer (or an admin) marked
// private, so the UI can offer "make public".
func (s *Server) annotatePrivate(actor authz.Actor) func(storage.FileInfo) storage.FileInfo {
	return func(item storage.FileInfo) storage.FileInfo {
		if s.authz == nil {
			return item
		}
		if owner, ok := s.authz.PrivateOwner(item.Path); ok && (actor.Admin || owner == actor.UserID) {
			item.Private = true
		}
		return item
	}
}

// handleMeta returns detailed metadata for a single path, optionally with
// a checksum (?checksum=md5|sha1|sha256|sha512).
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if !s.canRead(w, r, q.Get("path")) {
		return
	}
	info, err := s.store.Stat(r.Context(), q.Get("path"))
	if err != nil {
		respondErr(w, err)
		return
	}

	resp := struct {
		storage.FileInfo
		Checksums map[string]string `json:"checksums,omitempty"`
	}{FileInfo: info}

	if algo := q.Get("checksum"); algo != "" && !info.IsDir {
		h, err := newHasher(algo)
		if err != nil {
			apiError(w, http.StatusBadRequest, err.Error())
			return
		}
		sum, err := s.checksum(r.Context(), q.Get("path"), h)
		if err != nil {
			respondErr(w, err)
			return
		}
		resp.Checksums = map[string]string{algo: sum}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleUsage reports used and total bytes of the backing volume.
func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	usage, err := s.store.Usage(r.Context())
	if err != nil {
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

// handleCreateDir creates a new directory. Body: {"path": "..."}.
func (s *Server) handleCreateDir(w http.ResponseWriter, r *http.Request) {
	s.create(w, r, s.store.CreateDir)
}

// handleCreateFile creates a new empty file. Body: {"path": "..."}.
func (s *Server) handleCreateFile(w http.ResponseWriter, r *http.Request) {
	s.create(w, r, s.store.CreateFile)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, path string) (storage.FileInfo, error)) {
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if rejectReservedRoot(w, body.Path) {
		return
	}
	if !s.canWrite(w, r, body.Path) {
		return
	}
	info, err := fn(r.Context(), body.Path)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			apiError(w, http.StatusConflict, "already exists")
			return
		}
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, info)
}

// newHasher resolves a checksum algorithm name.
func newHasher(algo string) (hash.Hash, error) {
	switch algo {
	case "md5":
		return md5.New(), nil //nolint:gosec // integrity checksum, not cryptography
	case "sha1":
		return sha1.New(), nil //nolint:gosec // integrity checksum, not cryptography
	case "sha256":
		return sha256.New(), nil
	case "sha512":
		return sha512.New(), nil
	default:
		return nil, errors.New("unsupported checksum algorithm")
	}
}

// checksum streams the file through h and returns the hex digest.
func (s *Server) checksum(ctx context.Context, p string, h hash.Hash) (string, error) {
	rc, _, err := s.store.Open(ctx, p)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	if _, err := io.Copy(h, rc); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// decodeJSONBody decodes a size-limited JSON request body.
func decodeJSONBody(r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, limit)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// parseLimit reads a bounded integer limit from a query parameter.
func parseLimit(raw string, def, limit int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	if n > limit {
		return limit
	}
	return n
}
