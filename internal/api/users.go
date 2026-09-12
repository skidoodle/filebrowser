package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/skidoodle/filebrowser/internal/auth"
	"github.com/skidoodle/filebrowser/internal/authz"
	"github.com/skidoodle/filebrowser/internal/store"
)

// userJSON is the wire shape of an account (never the password hash).
type userJSON struct {
	ID         int64  `json:"id"`
	Username   string `json:"username"`
	Admin      bool   `json:"admin"`
	Scope      string `json:"scope"`
	IsOriginal bool   `json:"isOriginal"`
}

func toUserJSON(u store.User) userJSON {
	return userJSON{ID: u.ID, Username: u.Username, Admin: u.Admin, Scope: u.Scope, IsOriginal: u.IsOriginal}
}

// handleListUsers returns all accounts. Admin-only.
func (s *Server) handleListUsers(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Insecure {
		apiError(w, http.StatusBadRequest, "auth disabled in insecure mode")
		return
	}
	if s.auth == nil {
		apiError(w, http.StatusInternalServerError, "auth unavailable")
		return
	}
	users, err := s.auth.ListUsers()
	if err != nil {
		respondErr(w, err)
		return
	}
	out := make([]userJSON, 0, len(users))
	for _, u := range users {
		out = append(out, toUserJSON(u))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateUser creates an account. Admin-only.
// Body: {"username", "password", "admin", "scope"}.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Insecure {
		apiError(w, http.StatusBadRequest, "auth disabled in insecure mode")
		return
	}
	if s.auth == nil {
		apiError(w, http.StatusInternalServerError, "auth unavailable")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Admin    bool   `json:"admin"`
		Scope    string `json:"scope"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := s.auth.CreateUser(body.Username, body.Password, body.Admin, body.Scope)
	if err != nil {
		s.writeUserErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toUserJSON(u))
}

// handleUpdateUser changes an account's admin flag, scope or password
// (admin reset). Admin-only.
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Insecure {
		apiError(w, http.StatusBadRequest, "auth disabled in insecure mode")
		return
	}
	if s.auth == nil {
		apiError(w, http.StatusInternalServerError, "auth unavailable")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Username *string `json:"username"`
		Admin    *bool   `json:"admin"`
		Scope    *string `json:"scope"`
		Password *string `json:"password"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Password != nil && *body.Password != "" {
		if err := s.auth.AdminSetPassword(id, *body.Password); err != nil {
			s.writeUserErr(w, err)
			return
		}
	}
	if body.Username != nil {
		if _, err := s.auth.RenameUser(id, strings.TrimSpace(*body.Username)); err != nil {
			s.writeUserErr(w, err)
			return
		}
	}
	if s.patchUserAttributes(w, id, body.Admin, body.Scope) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// patchUserAttributes applies the admin flag and/or scope from a partial
// patch: only provided attributes change, so a rename never demotes an
// admin or wipes a scope. It writes the response when either attribute is
// present and returns true in that case.
func (s *Server) patchUserAttributes(w http.ResponseWriter, id int64, admin *bool, scope *string) bool {
	if admin == nil && scope == nil {
		return false
	}
	current, err := s.auth.User(id)
	if err != nil {
		s.writeUserErr(w, err)
		return true
	}
	newAdmin := current.Admin
	if admin != nil {
		newAdmin = *admin
	}
	newScope := current.Scope
	if scope != nil {
		newScope = *scope
	}
	u, err := s.auth.UpdateUser(id, newAdmin, newScope)
	if err != nil {
		s.writeUserErr(w, err)
		return true
	}
	writeJSON(w, http.StatusOK, toUserJSON(u))
	return true
}

// handleDeleteUser removes an account (original admin and last-admin
// guardrails apply). Admin-only.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Insecure {
		apiError(w, http.StatusBadRequest, "auth disabled in insecure mode")
		return
	}
	if s.auth == nil {
		apiError(w, http.StatusInternalServerError, "auth unavailable")
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.auth.DeleteUser(id); err != nil {
		s.writeUserErr(w, err)
		return
	}
	// User deletion cascades their private-folder rows away; the cached
	// set must not keep hiding those paths.
	if err := s.authz.Invalidate(); err != nil {
		s.log.Warn("user delete: invalidating private cache failed", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeUserErr maps account-management errors to statuses.
func (s *Server) writeUserErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrExists):
		apiError(w, http.StatusConflict, "username already taken")
	case errors.Is(err, store.ErrOriginal):
		apiError(w, http.StatusConflict, "the original admin cannot be deleted")
	case errors.Is(err, store.ErrLastAdmin):
		apiError(w, http.StatusConflict, "cannot remove the last admin")
	case errors.Is(err, store.ErrNotFound):
		apiError(w, http.StatusNotFound, "user not found")
	case errors.Is(err, store.ErrInvalid), errors.Is(err, auth.ErrBadUsername),
		errors.Is(err, auth.ErrWeakPassword):
		apiError(w, http.StatusBadRequest, "invalid username, scope or password (min 8 characters)")
	default:
		respondErr(w, err)
	}
}

// handleAuthPassword lets a signed-in user change their own username and
// password. Username changes apply to any account (sessions are id-bound
// and survive); password changes require the current password.
// Body: {"username"?, "current", "password"}.
func (s *Server) handleAuthPassword(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Insecure {
		apiError(w, http.StatusBadRequest, "auth disabled in insecure mode")
		return
	}
	if s.auth == nil {
		apiError(w, http.StatusInternalServerError, "auth unavailable")
		return
	}
	id := requestIdentity(r).UserID
	if id == 0 {
		apiError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body struct {
		Username string `json:"username"`
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.Password != "" {
		if err := s.auth.ChangePassword(id, body.Current, body.Password); err != nil {
			switch {
			case errors.Is(err, auth.ErrInvalidCredentials):
				apiError(w, http.StatusForbidden, "current password is wrong")
			case errors.Is(err, auth.ErrWeakPassword):
				apiError(w, http.StatusBadRequest, "password too short (minimum 8 characters)")
			default:
				apiError(w, http.StatusInternalServerError, "internal error")
			}
			return
		}
	}

	if u := strings.TrimSpace(body.Username); u != "" {
		current, err := s.auth.User(id)
		if err != nil {
			s.writeUserErr(w, err)
			return
		}
		if u != current.Username {
			if _, err := s.auth.RenameUser(id, u); err != nil {
				s.writeUserErr(w, err)
				return
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSetPrivate marks a directory private: hidden from everyone except
// its owner and admins. Scoped users may only privatize inside their own
// scope; admins anywhere. Body: {"path"}.
func (s *Server) handleSetPrivate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id := requestIdentity(r)
	if !s.cfg.Insecure && id.UserID == 0 {
		apiError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !s.cfg.Insecure && !id.Admin && !authz.InScope(id.Scope, body.Path) {
		apiError(w, http.StatusForbidden, "path outside your scope")
		return
	}
	if err := s.authz.SetPrivate(toActor(id), body.Path); err != nil {
		switch {
		case errors.Is(err, authz.ErrOutOfScope), errors.Is(err, authz.ErrDenied):
			apiError(w, http.StatusForbidden, "path outside your scope")
		case errors.Is(err, authz.ErrPrivate):
			apiError(w, http.StatusNotFound, "not found")
		default:
			respondErr(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleUnsetPrivate clears the private mark on a directory (owner or
// admin). Query: path.
func (s *Server) handleUnsetPrivate(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		apiError(w, http.StatusBadRequest, "missing path")
		return
	}
	id := requestIdentity(r)
	if err := s.authz.UnsetPrivate(toActor(id), path); err != nil {
		switch {
		case errors.Is(err, authz.ErrPrivate):
			apiError(w, http.StatusNotFound, "not found")
		case errors.Is(err, authz.ErrOutOfScope), errors.Is(err, authz.ErrDenied):
			apiError(w, http.StatusForbidden, "path outside your scope")
		default:
			respondErr(w, err)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
