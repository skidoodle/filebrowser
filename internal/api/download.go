package api

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/skidoodle/filebrowser/internal/storage"
)

// handleDownload streams directories or multi-selections as archives.
// Query: path (base directory), files (comma-separated entry names within
// the base directory; empty = the whole directory), algo (zip | tar.gz).
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	algo := q.Get("algo")
	if algo == "" {
		algo = "zip"
	}
	if algo != "zip" && algo != "tar.gz" {
		apiError(w, http.StatusBadRequest, "unsupported archive format")
		return
	}

	wanted, err := parseDownloadSelection(q.Get("files"))
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}

	base := q.Get("path")
	if !s.canRead(w, r, base) {
		return
	}
	baseInfo, err := s.store.Stat(r.Context(), base)
	if err != nil {
		respondErr(w, err)
		return
	}
	if !baseInfo.IsDir {
		// Single-file downloads belong to /api/raw. The target is always a
		// relative storage path, never an attacker-controlled absolute URL.
		target := url.URL{
			Path:     "/api/raw",
			RawQuery: url.Values{"path": {base}}.Encode(),
		}
		http.Redirect(w, r, target.String(), http.StatusFound)
		return
	}

	archiveName := downloadArchiveName(base, wanted)

	w.Header().Set("Content-Disposition",
		mime.FormatMediaType("attachment", map[string]string{"filename": archiveName + "." + algo}))
	// Never let archived uploaded content execute while streaming.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")

	if algo == "zip" {
		s.streamZip(w, r, base, wanted)
		return
	}
	s.streamTarGz(w, r, base, wanted)
}

// parseDownloadSelection validates the comma-separated file list.
// Empty input means "archive the whole directory".
func parseDownloadSelection(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	wanted := strings.Split(raw, ",")
	for _, name := range wanted {
		if name == "" || strings.ContainsAny(name, "/\\") || name == ".." || name == "." {
			return nil, errors.New("invalid selection")
		}
	}
	return wanted, nil
}

// downloadArchiveName derives the attachment name for an archive.
func downloadArchiveName(base string, wanted []string) string {
	switch {
	case len(wanted) == 1:
		return sanitizeArchiveName(wanted[0])
	case base == "." || base == "":
		return "filebrowser"
	default:
		return sanitizeArchiveName(path.Base(base))
	}
}

// archiveEntry is one member to write into an archive.
type archiveEntry struct {
	path    string // storage path to open
	name    string // name inside the archive (slash-separated, clean)
	size    int64
	modTime time.Time
	mode    fs.FileMode
	isDir   bool
}

// archiveRoot is one input subtree of an archive download.
type archiveRoot struct {
	rel  string // storage path to walk
	name string // member name inside the archive ("" = whole base dir)
}

// collectEntries resolves the archive input: the whole base dir or the
// selected entries, mapped to archive-relative names.
func (s *Server) collectEntries(r *http.Request, base string, wanted []string) ([]archiveEntry, error) {
	if base == "." || base == "" {
		base = "."
	}

	var roots []archiveRoot
	if len(wanted) == 0 {
		roots = []archiveRoot{{rel: base, name: ""}}
	} else {
		for _, name := range wanted {
			rel := name
			if base != "." {
				rel = base + "/" + name
			}
			roots = append(roots, archiveRoot{rel: rel, name: name})
		}
	}

	var entries []archiveEntry
	actor := s.readerActor(r)
	hidden := func(p string) bool {
		return s.authz != nil && !s.cfg.Insecure && s.authz.CanRead(actor, p) != nil
	}
	for _, rt := range roots {
		if hidden(rt.rel) {
			continue
		}
		if err := s.walkArchiveRoot(r, rt, hidden, &entries); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// walkArchiveRoot walks one archive input root, skipping hidden entries.
func (s *Server) walkArchiveRoot(r *http.Request, rt archiveRoot, hidden func(string) bool, entries *[]archiveEntry) error {
	return s.store.Walk(r.Context(), rt.rel, storage.SearchOptions{}, func(fi storage.FileInfo) error {
		if hidden(fi.Path) {
			return nil
		}
		*entries = append(*entries, archiveEntry{
			path:    fi.Path,
			name:    cleanArchiveName(archiveMemberName(fi.Path, rt)),
			size:    fi.Size,
			modTime: fi.ModTime,
			mode:    fi.Mode,
			isDir:   fi.IsDir,
		})
		return nil
	})
}

// archiveMemberName maps a storage path to its archive-relative name.
func archiveMemberName(p string, rt archiveRoot) string {
	below := ""
	switch {
	case p == rt.rel:
		below = ""
	case strings.HasPrefix(p, rt.rel+"/"):
		below = p[len(rt.rel)+1:]
	default:
		below = p
	}
	if rt.name == "" {
		return below
	}
	if below == "" {
		return rt.name
	}
	return rt.name + "/" + below
}

// streamZip writes a streaming zip; sizes are unknown up front, so the
// writer emits data descriptors and never buffers whole files.
func (s *Server) streamZip(w http.ResponseWriter, r *http.Request, base string, wanted []string) {
	entries, err := s.collectEntries(r, base, wanted)
	if err != nil {
		respondErr(w, err)
		return
	}

	zw := zip.NewWriter(w)
	ok := true
	for _, e := range entries {
		if e.isDir {
			continue // zip does not need explicit directory entries
		}
		h := &zip.FileHeader{Name: e.name, Modified: e.modTime}
		h.SetMode(e.mode)
		f, err := zw.CreateHeader(h)
		if err != nil {
			ok = false
			break
		}
		rc, _, err := s.store.Open(r.Context(), e.path)
		if err != nil {
			ok = false
			break
		}
		_, err = io.Copy(f, rc)
		_ = rc.Close()
		if err != nil {
			ok = false
			break
		}
	}
	// Close finalizes the archive (central directory) and must not be
	// ignored: its error is how clients detect truncated transfers.
	if err := zw.Close(); err != nil {
		ok = false
	}
	if !ok {
		s.log.Warn("archive streaming aborted", "algo", "zip", "base", base)
	}
}

// streamTarGz writes a gzipped tar; tar headers carry the known sizes.
func (s *Server) streamTarGz(w http.ResponseWriter, r *http.Request, base string, wanted []string) {
	entries, err := s.collectEntries(r, base, wanted)
	if err != nil {
		respondErr(w, err)
		return
	}

	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	ok := true
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			ModTime:  e.modTime,
			Mode:     int64(e.mode.Perm()),
			Typeflag: tar.TypeReg,
		}
		if e.isDir {
			hdr.Typeflag = tar.TypeDir
			hdr.Name += "/"
		} else {
			hdr.Size = e.size
		}
		if err := tw.WriteHeader(hdr); err != nil {
			ok = false
			break
		}
		if e.isDir {
			continue
		}
		rc, _, err := s.store.Open(r.Context(), e.path)
		if err != nil {
			ok = false
			break
		}
		_, err = io.Copy(tw, rc)
		_ = rc.Close()
		if err != nil {
			ok = false
			break
		}
	}
	if err := tw.Close(); err != nil {
		ok = false
	}
	if err := gz.Close(); err != nil {
		ok = false
	}
	if !ok {
		s.log.Warn("archive streaming aborted", "algo", "tar.gz", "base", base)
	}
}

// cleanArchiveName normalizes a member name for safe extraction: forward
// slashes only, no traversal — the zip-slip defense.
func cleanArchiveName(name string) string {
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "\x00", "_")
	name = path.Clean("/" + name)
	return strings.TrimPrefix(name, "/")
}

func sanitizeArchiveName(name string) string {
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" || name == "." {
		return "filebrowser"
	}
	return name
}
