package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/skidoodle/filebrowser/internal/auth"
	"github.com/skidoodle/filebrowser/internal/authz"
)

// identityKey is the context key the write/admin middleware uses to pass
// the resolved identity to handlers.
type identityKey struct{}

// Access policy values ("public", "readonly", "private").
const (
	policyPublic   = "public"
	policyReadonly = "readonly"
	policyPrivate  = "private"
)

// insecureIdentity is the identity granted to every caller when the
// server runs in insecure (auth-disabled) mode.
func insecureIdentity() auth.Identity {
	return auth.Identity{Username: "insecure", Admin: true}
}

// withIdentity stores id in the request context.
func withIdentity(r *http.Request, id auth.Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey{}, id))
}

// requestIdentity returns the identity stashed by middleware, or the
// guest identity for requests that bypassed it (reads).
func requestIdentity(r *http.Request) auth.Identity {
	if id, ok := r.Context().Value(identityKey{}).(auth.Identity); ok {
		return id
	}
	return auth.Guest
}

// readerActor resolves the actor for read endpoints, which have no
// identity middleware: stashed identity, else cookie, else guest.
func (s *Server) readerActor(r *http.Request) authz.Actor {
	if id, ok := r.Context().Value(identityKey{}).(auth.Identity); ok {
		return toActor(id)
	}
	if s.cfg.Insecure {
		return toActor(insecureIdentity())
	}
	if s.auth != nil {
		if id, err := s.identity(r); err == nil {
			return toActor(id)
		}
	}
	return authz.Actor{}
}

// toActor converts an auth identity into an authz actor.
func toActor(id auth.Identity) authz.Actor {
	return authz.Actor{UserID: id.UserID, Admin: id.Admin, Scope: id.Scope}
}

// accessPolicy returns the effective access policy ("public", "readonly", "private").
// Priority: SQLite meta table > FILEBROWSER_ACCESS_POLICY config > default "public".
func (s *Server) accessPolicy() string {
	if s.appStore != nil {
		p, err := s.appStore.AccessPolicy(s.cfg.AccessPolicy)
		if err == nil && p != "" {
			return p
		}
	}
	if s.cfg != nil && s.cfg.AccessPolicy != "" {
		return s.cfg.AccessPolicy
	}
	return policyPublic
}

// handleMe reports the caller's capabilities: admin status, username and
// scope, access policy, and whether onboarding is pending.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	resp := struct {
		Admin        bool   `json:"admin"`
		Insecure     bool   `json:"insecure"`
		Initialized  bool   `json:"initialized"`
		Username     string `json:"username,omitempty"`
		Scope        string `json:"scope,omitempty"`
		AccessPolicy string `json:"access_policy"`
	}{
		Insecure:     s.cfg.Insecure,
		Initialized:  true,
		AccessPolicy: s.accessPolicy(),
	}

	if s.cfg.Insecure {
		resp.Admin = true
	} else if s.auth != nil {
		resp.Initialized = s.auth.Initialized()
		if id, err := s.identity(r); err == nil {
			resp.Admin = id.Admin
			resp.Username = id.Username
			resp.Scope = id.Scope
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleAuthSetup creates the admin account; only allowed while
// uninitialized. It logs the caller in on success.
func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Insecure {
		apiError(w, http.StatusBadRequest, "auth disabled in insecure mode")
		return
	}
	if s.auth == nil {
		apiError(w, http.StatusInternalServerError, "auth unavailable")
		return
	}
	if s.auth.Initialized() {
		apiError(w, http.StatusConflict, "admin account already exists")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := s.auth.Setup(body.Password); err != nil {
		switch {
		case errors.Is(err, auth.ErrWeakPassword):
			apiError(w, http.StatusBadRequest, "password too short (minimum 8 characters)")
		case errors.Is(err, auth.ErrAlreadyExists):
			apiError(w, http.StatusConflict, "admin account already exists")
		default:
			apiError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}
	s.log.Info("admin account created via onboarding")
	token, err := s.auth.SessionAfterSetup()
	if err != nil {
		apiError(w, http.StatusInternalServerError, "internal error")
		return
	}
	s.setSession(w, r, token)
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthLogin verifies the password and sets the session cookie.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
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
	}
	if err := decodeJSONBody(r, &body, 4<<10); err != nil {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ip := auth.ClientIP(remoteAddr(r)).String()
	token, err := s.auth.Login(ip, body.Username, body.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrLocked):
			apiError(w, http.StatusTooManyRequests, "too many failed attempts")
		case errors.Is(err, auth.ErrUninitialized):
			apiError(w, http.StatusConflict, "no accounts exist yet")
		default:
			apiError(w, http.StatusUnauthorized, "invalid credentials")
		}
		return
	}
	s.setSession(w, r, token)
	w.WriteHeader(http.StatusNoContent)
}

// handleAuthLogout clears the session cookie.
func (s *Server) handleAuthLogout(w http.ResponseWriter, _ *http.Request) {
	clearSession(w)
	w.WriteHeader(http.StatusNoContent)
}

// identity resolves the caller from the session cookie. Insecure mode
// grants admin to everyone.
func (s *Server) identity(r *http.Request) (auth.Identity, error) {
	if s.cfg.Insecure {
		return insecureIdentity(), nil
	}
	if s.auth == nil {
		return auth.Identity{}, auth.ErrInvalidCredentials
	}
	c, err := r.Cookie(auth.SessionCookieName())
	if err != nil || c.Value == "" {
		return auth.Identity{}, auth.ErrInvalidCredentials
	}
	return s.auth.VerifySession(c.Value)
}

// authenticate resolves the caller for mutating endpoints. Insecure mode
// grants admin to everyone. Returns (identity, ok).
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (auth.Identity, bool) {
	if s.cfg.Insecure {
		return insecureIdentity(), true
	}
	if s.auth == nil {
		apiError(w, http.StatusInternalServerError, "auth unavailable")
		return auth.Guest, false
	}
	id, err := s.identity(r)
	if err != nil {
		apiError(w, http.StatusUnauthorized, "authentication required")
		return auth.Guest, false
	}
	if !sameOrigin(r) {
		apiError(w, http.StatusForbidden, "cross-origin request rejected")
		return auth.Guest, false
	}
	return id, true
}

// writeCreation wraps creation endpoints (POST /api/dir, POST /api/file, POST /api/tus):
// allows authenticated sessions, or anonymous visitors when access policy is "public".
func (s *Server) writeCreation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !sameOrigin(r) {
			apiError(w, http.StatusForbidden, "cross-origin request rejected")
			return
		}
		if s.cfg.Insecure {
			next.ServeHTTP(w, withIdentity(r, insecureIdentity()))
			return
		}
		if s.auth != nil {
			if id, err := s.identity(r); err == nil {
				next.ServeHTTP(w, withIdentity(r, id))
				return
			}
		}
		if s.accessPolicy() != policyPublic {
			apiError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, withIdentity(r, auth.Guest))
	})
}

// write wraps mutation handlers: it requires an authenticated session (or
// insecure mode), rejects cross-origin requests, and stashes the identity
// in the request context for the handler's per-path permission checks.
func (s *Server) write(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		next.ServeHTTP(w, withIdentity(r, id))
	})
}

// admin wraps admin-only handlers: authentication plus the admin flag.
func (s *Server) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		if !id.Admin {
			apiError(w, http.StatusForbidden, "admin required")
			return
		}
		next.ServeHTTP(w, withIdentity(r, id))
	})
}

// canWrite enforces write permission on a target path inside a mutation
// handler. Mapping: anonymous → 401 (unless policy is public and target is public),
// out-of-scope → 403, someone else's private folder → 404 (hidden, never 403).
func (s *Server) canWrite(w http.ResponseWriter, r *http.Request, path string) bool {
	if s.cfg.Insecure {
		return true
	}
	actor := toActor(requestIdentity(r))
	if actor.UserID == 0 {
		return s.canWriteAnonymous(w, path)
	}
	if s.authz == nil {
		apiError(w, http.StatusInternalServerError, "authz unavailable")
		return false
	}
	err := s.authz.CanWrite(actor, path)
	switch {
	case err == nil:
		return true
	case errors.Is(err, authz.ErrDenied):
		apiError(w, http.StatusUnauthorized, "authentication required")
	case errors.Is(err, authz.ErrOutOfScope):
		apiError(w, http.StatusForbidden, "path outside your scope")
	case errors.Is(err, authz.ErrPrivate):
		apiError(w, http.StatusNotFound, "not found")
	default:
		respondErr(w, err)
	}
	return false
}

// canWriteAnonymous authorizes an anonymous write: allowed only when the
// access policy is public and the target is not inside someone else's
// private folder.
func (s *Server) canWriteAnonymous(w http.ResponseWriter, path string) bool {
	if s.accessPolicy() != policyPublic {
		apiError(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if s.authz == nil {
		return true
	}
	err := s.authz.CanRead(authz.Actor{}, path)
	switch {
	case err == nil:
		return true
	case errors.Is(err, authz.ErrPrivate):
		apiError(w, http.StatusNotFound, "not found")
	default:
		respondErr(w, err)
	}
	return false
}

// canRead enforces read visibility on a path: in private access policy,
// anonymous callers get 401; private folders of others answer 404 so their
// existence is never revealed.
func (s *Server) canRead(w http.ResponseWriter, r *http.Request, path string) bool {
	if s.cfg.Insecure {
		return true
	}
	actor := s.readerActor(r)
	if s.accessPolicy() == policyPrivate && actor.UserID == 0 && !actor.Admin {
		apiError(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	if s.authz == nil {
		return true
	}
	err := s.authz.CanRead(actor, path)
	if err == nil {
		return true
	}
	if errors.Is(err, authz.ErrPrivate) {
		apiError(w, http.StatusNotFound, "not found")
		return false
	}
	respondErr(w, err)
	return false
}

// pathID parses a {id} path value as int64, answering 400 otherwise.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		apiError(w, http.StatusBadRequest, "invalid user id")
		return 0, false
	}
	return id, true
}

// sameOrigin verifies the request is same-origin, mitigating CSRF for
// cookie-authenticated mutations.
//
// Sec-Fetch-Site is computed by the browser from the URLs it sees and
// cannot be spoofed by JavaScript, so it is authoritative — including
// behind proxies that rewrite the Host header (Vite dev proxy, reverse
// proxies), where an Origin/Host comparison would wrongly fail.
func sameOrigin(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "same-origin") {
		return true
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	}
	// No Origin and no fetch metadata: non-browser client (curl, scripts).
	return r.Header.Get("Sec-Fetch-Mode") == ""
}

// setSession writes the session cookie with sliding expiry. Secure is set
// for https requests so the token never travels over plain HTTP.
func (s *Server) setSession(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: HttpOnly and SameSite are set; Secure depends on the scheme
		Name:     auth.SessionCookieName(),
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(auth.SessionTTL()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSession expires the session cookie.
func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: HttpOnly and SameSite are set; clearing an already-expired cookie
		Name:     auth.SessionCookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// remoteAddr returns the request's remote address (guard-stripped proxies
// do not affect the login backoff, which is best-effort).
func remoteAddr(r *http.Request) string { return r.RemoteAddr }
