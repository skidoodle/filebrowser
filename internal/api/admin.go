package api

import (
	"errors"
	"net/http"
	"os"
)

// maxDeleteBatch bounds how many paths a single delete request may carry.
const maxDeleteBatch = 1000

// handleDelete removes files or directories (recursively). Admin-only.
// Body: {"paths": ["a", "b", ...]}.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := decodeJSONBody(r, &body, 64<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Paths) == 0 || len(body.Paths) > maxDeleteBatch {
		apiError(w, http.StatusBadRequest, "paths must contain 1 to 1000 entries")
		return
	}
	deleted := make([]string, 0, len(body.Paths))
	for _, p := range body.Paths {
		if !s.canWrite(w, r, p) {
			return
		}
		// RemoveAll-style deletes hide typos; report missing paths as 404.
		if _, err := s.store.Stat(r.Context(), p); err != nil {
			if len(deleted) > 0 {
				writeJSON(w, http.StatusMultiStatus, map[string]any{"deleted": deleted, "failed": p})
				return
			}
			respondErr(w, err)
			return
		}
		if err := s.store.Remove(r.Context(), p); err != nil {
			if len(deleted) > 0 {
				// Partial success is reported as such; the frontend can retry
				// the remainder.
				writeJSON(w, http.StatusMultiStatus, map[string]any{"deleted": deleted, "failed": p})
				return
			}
			respondErr(w, err)
			return
		}
		// Deleted directories take their private marks with them; failures
		// are cosmetic (the folder is gone either way).
		if err := s.authz.DeletePrivateBelow(p); err != nil {
			s.log.Warn("delete: clearing private marks failed", "path", p, "err", err)
		}
		deleted = append(deleted, p)
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleMove renames or relocates a single entry. Admin-only.
// Body: {"from": "a", "to": "b"}.
func (s *Server) handleMove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.From == "" || body.To == "" {
		apiError(w, http.StatusBadRequest, "from and to are required")
		return
	}
	if rejectReservedRoot(w, body.To) {
		return
	}
	if !s.canWrite(w, r, body.From) || !s.canWrite(w, r, body.To) {
		return
	}
	info, err := s.store.Move(r.Context(), body.From, body.To)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrExist):
			apiError(w, http.StatusConflict, "destination already exists")
		case errors.Is(err, os.ErrInvalid):
			apiError(w, http.StatusBadRequest, "invalid move")
		default:
			respondErr(w, err)
		}
		return
	}
	// A moved private folder keeps its privacy under the new name.
	if err := s.authz.RenamePrivate(body.From, body.To); err != nil {
		s.log.Warn("move: rewriting private marks failed", "from", body.From, "to", body.To, "err", err)
	}
	writeJSON(w, http.StatusOK, info)
}

// handleSave streams the request body over an existing or new file.
// Admin-only. Query: path. The body is size-capped like an upload.
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		apiError(w, http.StatusBadRequest, "missing path")
		return
	}
	if rejectReservedRoot(w, path) {
		return
	}
	if !s.canWrite(w, r, path) {
		return
	}
	if r.Body == nil {
		apiError(w, http.StatusBadRequest, "missing body")
		return
	}
	limited := http.MaxBytesReader(w, r.Body, s.cfg.MaxUpload)
	info, err := s.store.Write(r.Context(), path, limited, s.cfg.MaxUpload)
	if err != nil {
		if tooLarge, ok := errors.AsType[*http.MaxBytesError](err); ok {
			_ = tooLarge
			apiError(w, http.StatusRequestEntityTooLarge, "content exceeds maximum size")
			return
		}
		respondErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
