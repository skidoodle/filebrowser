package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/skidoodle/filebrowser/internal/guard"
)

// handleCapability issues capability tokens to browser clients.
// GET returns either a ready token or a proof-of-work challenge for
// suspicious clients; POST verifies a solved challenge and returns the token.
func (s *Server) handleCapability(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleCapabilityGet(w, r)

	case http.MethodPost:
		s.handleCapabilityPost(w, r)

	default:
		apiError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleCapabilityGet answers token requests, serving a PoW challenge to
// scripted clients when the difficulty is nonzero.
func (s *Server) handleCapabilityGet(w http.ResponseWriter, r *http.Request) {
	// Scripted clients with bot-ish user agents must pay a small PoW
	// toll first; real browsers sail through invisibly.
	if s.guard != nil && s.guard.PowDifficulty() > 0 && guard.SuspiciousUserAgent(r.UserAgent()) {
		challenge, err := guard.RandomChallenge()
		if err != nil {
			apiError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"challenge":  challenge,
			"difficulty": s.guard.PowDifficulty(),
		})
		return
	}

	s.issueCapability(w)
}

// handleCapabilityPost verifies a solved proof-of-work.
func (s *Server) handleCapabilityPost(w http.ResponseWriter, r *http.Request) {
	if s.guard == nil {
		s.issueCapability(w)
		return
	}

	var body struct {
		Challenge string `json:"challenge"`
		Nonce     string `json:"nonce"`
	}

	if err := decodeJSONBody(r, &body, 4<<10); err != nil || body.Challenge == "" {
		apiError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sum := sha256.Sum256([]byte(body.Challenge + ":" + body.Nonce))
	if !guard.CheckDifficulty(hex.EncodeToString(sum[:]), s.guard.PowDifficulty()) {
		apiError(w, http.StatusForbidden, "invalid proof of work")
		return
	}

	s.issueCapability(w)
}

func (s *Server) issueCapability(w http.ResponseWriter) {
	if s.guard == nil {
		// Guard disabled: nothing to enforce, answer for API symmetry.
		writeJSON(w, http.StatusOK, map[string]string{"capability": ""})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"capability": s.guard.Tokens.Issue(time.Hour),
	})
}
