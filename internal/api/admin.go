package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/skidoodle/filebrowser/internal/authz"
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
// Admin/writer or valid short-lived edit token. Query: path.
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		apiError(w, http.StatusBadRequest, "missing path")
		return
	}
	if rejectReservedRoot(w, path) {
		return
	}
	if !sameOrigin(r) {
		apiError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}

	r, ok := s.authorizeSave(w, r, path)
	if !ok {
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

// authorizeSave authorizes a write to path: either a valid edit token
// minted for that path (anonymous, public policy) or an authenticated
// writer session. Returns the request, possibly with the session
// identity stashed in its context, and whether the write may proceed.
func (s *Server) authorizeSave(w http.ResponseWriter, r *http.Request, path string) (*http.Request, bool) {
	editToken := r.Header.Get("X-Edit-Token")
	if editToken != "" && s.accessPolicy() == policyPublic && s.tokens != nil && s.tokens.VerifyForPath(editToken, path) {
		if s.authz != nil && !s.cfg.Insecure && s.authz.CanRead(authz.Actor{}, path) != nil {
			apiError(w, http.StatusNotFound, "not found")
			return r, false
		}
		return r, true
	}
	id, ok := s.authenticate(w, r)
	if !ok {
		return r, false
	}
	r = withIdentity(r, id)
	return r, s.canWrite(w, r, path)
}

// handleGetPolicy returns the active access policy ("public", "readonly", "private"). Admin-only.
func (s *Server) handleGetPolicy(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"access_policy": s.accessPolicy(),
	})
}

// handleSetPolicy updates the active access policy. Admin-only.
// Body: {"access_policy": "public" | "readonly" | "private"}.
func (s *Server) handleSetPolicy(w http.ResponseWriter, r *http.Request) {
	if s.appStore == nil {
		apiError(w, http.StatusInternalServerError, "store unavailable")
		return
	}
	var body struct {
		AccessPolicy string `json:"access_policy"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	policy := strings.ToLower(strings.TrimSpace(body.AccessPolicy))
	switch policy {
	case policyPublic, policyReadonly, policyPrivate:
		// valid
	default:
		apiError(w, http.StatusBadRequest, "invalid access policy: must be public, readonly, or private")
		return
	}
	if err := s.appStore.SetAccessPolicy(policy); err != nil {
		respondErr(w, err)
		return
	}
	s.log.Info("access policy updated", "access_policy", policy)
	writeJSON(w, http.StatusOK, map[string]string{
		"access_policy": policy,
	})
}
