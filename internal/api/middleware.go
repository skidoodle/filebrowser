package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"time"
)

// writeJSON serializes v as a JSON response body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// apiError writes a structured error response.
func apiError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// respondErr maps well-known storage errors to HTTP statuses so handlers
// can simply pass their error through.
func respondErr(w http.ResponseWriter, err error) {
	if he, ok := errors.AsType[*httpError](err); ok {
		apiError(w, he.status, he.msg)
		return
	}
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, fs.ErrNotExist):
		apiError(w, http.StatusNotFound, "not found")
	case errors.Is(err, os.ErrExist):
		apiError(w, http.StatusConflict, "already exists")
	case errors.Is(err, os.ErrPermission):
		apiError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, os.ErrInvalid):
		apiError(w, http.StatusBadRequest, "invalid path")
	default:
		apiError(w, http.StatusInternalServerError, "internal error")
	}
}

// recoverMiddleware converts handler panics into 500 responses.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered", "method", r.Method, "path", r.URL.Path, "panic", rec)
				apiError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logMiddleware emits one structured line per request.
func (s *Server) logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		s.log.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start).Round(time.Millisecond),
		)
	})
}

// secureHeadersMiddleware sets conservative default headers on every response.
func (s *Server) secureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// statusWriter records the response status for logging and forwards the
// optional interfaces handlers rely on: Flusher (streaming search) and the
// Unwrap hook used by http.ResponseController.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.status = http.StatusOK
		w.wrote = true
	}
	return w.ResponseWriter.Write(b)
}

// Flush forwards to the wrapped writer so streaming handlers can flush.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the wrapped writer to http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
