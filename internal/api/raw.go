package api

import (
	"fmt"
	"mime"
	"net/http"

	"github.com/skidoodle/filebrowser/internal/storage"
)

// handleRaw streams a file for download or inline viewing.
// It uses http.ServeContent, which provides HTTP Range (resumable
// downloads), If-Modified-Since and ETag handling for free.
// Query: path, inline (force inline/attachment).
func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if !s.canRead(w, r, q.Get("path")) {
		return
	}
	rc, info, err := s.store.Open(r.Context(), q.Get("path"))
	if err != nil {
		respondErr(w, err)
		return
	}
	defer rc.Close()
	if info.IsDir {
		apiError(w, http.StatusBadRequest, "cannot download a directory as a single file")
		return
	}

	h := w.Header()
	// Uploaded content must never execute script when navigated to directly.
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	// Serve the detected MIME, not the filename-derived one: ServeContent
	// falls back to the OS mime table, which on Windows maps formats like
	// .mkv/.ac3/.ai to registry entries that browsers handle worse (or not
	// at all - .ai renders as PDF only when labeled application/pdf).
	h.Set("Content-Type", info.MimeType)
	// Always revalidate, with a strong ETag: Last-Modified only has
	// second granularity, so a save landing in the same second as a
	// cached response would otherwise be answered with 304 and a stale
	// body forever. The ETag (modtime nanos + size) catches same-second
	// changes while still allowing cheap 304s for unchanged files.
	h.Set("Cache-Control", "no-cache")
	h.Set("ETag", fmt.Sprintf(`"%x-%x"`, info.ModTime.UnixNano(), info.Size))

	// Charge the payload against the client's download budget.
	if s.guard != nil && !s.guard.AllowDownload(r, info.Size) {
		w.Header().Set("Retry-After", "1")
		apiError(w, http.StatusTooManyRequests, "download limit exceeded")
		return
	}

	disposition := "attachment"
	if q.Get("inline") == "true" || isInlineType(info.Type) {
		disposition = "inline"
	}
	if cd := mime.FormatMediaType(disposition, map[string]string{"filename": info.Name}); cd != "" {
		h.Set("Content-Disposition", cd)
	}

	http.ServeContent(w, r, info.Name, info.ModTime, rc)
}

// isInlineType reports whether the given type is safe and useful to render
// inline in a browser. Everything else downloads as an attachment.
func isInlineType(t storage.FileType) bool {
	switch t {
	case storage.TypeVideo, storage.TypeAudio, storage.TypeImage, storage.TypePDF, storage.TypeText:
		return true
	case storage.TypeDir, storage.TypeBlob:
		return false
	default:
		return false
	}
}
