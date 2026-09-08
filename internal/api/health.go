package api

import "net/http"

// handleHealth reports server liveness and build metadata for monitoring
// and the frontend.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": s.version,
		"commit":  s.commit,
	})
}
