package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skidoodle/filebrowser/internal/storage"
)

// tusChunkBuffer bounds the in-memory size of PATCH body staging.
const tusChunkBuffer = 1 << 20

// tusDrainLimit bounds how much of an unconsumed PATCH body is read before
// answering an early rejection (stale offset, unknown session, …). Reading
// instead of leaving data unread keeps the connection clean: Go resets TCP
// connections closed with unread request data, browsers report that as
// ERR_CONNECTION_RESET, and the tus client loses the status code it needs.
// The bundled SPA chunks at 16 MiB, so 32 MiB covers any legitimate client;
// senders beyond that are hostile and still get reset.
const tusDrainLimit = 32 << 20

// drainTusBody discards an unconsumed PATCH body before an error response.
func drainTusBody(r *http.Request) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, tusDrainLimit))
}

// tusSession is one in-progress tus upload.
type tusSession struct {
	id       string
	path     string // storage-relative target path
	size     int64  // declared Upload-Length
	up       storage.Upload
	mu       sync.Mutex
	lastUsed time.Time
}

// tusRegistry tracks in-memory upload sessions. The authoritative offset is
// always the on-disk file size, so uploads survive server restarts even
// though sessions do not; the registry only accelerates lookups and drives
// garbage collection of abandoned partials.
type tusRegistry struct {
	store    storage.Storage
	log      *slog.Logger
	mu       sync.Mutex
	sessions map[string]*tusSession
	maxSize  int64
	ttl      time.Duration
}

func newTusRegistry(store storage.Storage, log *slog.Logger, maxSize int64, ttl time.Duration) *tusRegistry {
	r := &tusRegistry{
		store:    store,
		log:      log,
		sessions: make(map[string]*tusSession),
		maxSize:  maxSize,
		ttl:      ttl,
	}
	go r.gcLoop()
	return r
}

func (r *tusRegistry) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		r.gcOnce()
	}
}

func (r *tusRegistry) gcOnce() {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	for id, s := range r.sessions {
		if now.Sub(s.lastUsed) > r.ttl {
			_ = s.up.Abort() // removes the partial file
			delete(r.sessions, id)
			r.log.Info("discarded abandoned upload", "path", s.path)
		}
	}
}

func (r *tusRegistry) get(id string) (*tusSession, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if ok {
		s.lastUsed = time.Now()
	}
	return s, ok
}

func (r *tusRegistry) add(s *tusSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.id] = s
}

func (r *tusRegistry) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
}

// tusID returns a random URL-safe session id.
func tusID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// tusCommon sets the mandatory Tus-Resumable header on every response.
func tusCommon(w http.ResponseWriter) {
	w.Header().Set("Tus-Resumable", "1.0")
}

// handleTusOptions answers protocol capability discovery.
func (s *Server) handleTusOptions(w http.ResponseWriter, _ *http.Request) {
	tusCommon(w)
	h := w.Header()
	h.Set("Tus-Version", "1.0.0")
	h.Set("Tus-Extension", "creation,termination")
	h.Set("Tus-Max-Size", strconv.FormatInt(s.cfg.MaxUpload, 10))
	w.WriteHeader(http.StatusNoContent)
}

// handleTusCreate implements the tus creation extension: a POST with
// Upload-Length starts a new upload writing to the target path. The target
// comes from ?path= or the Upload-Metadata "path" field.
// Existing files are never clobbered unless ?override=true is set.
func (s *Server) handleTusCreate(w http.ResponseWriter, r *http.Request) {
	tusCommon(w)

	meta := parseTusMetadata(r.Header.Get("Upload-Metadata"))
	target, length, ok := s.resolveTusTarget(w, r, meta)
	if !ok {
		return
	}

	// Uploads are writes: admins anywhere, scoped users inside their scope,
	// anonymous guests never.
	exists, err := s.targetExists(r.Context(), target)
	if err != nil {
		respondErr(w, err)
		return
	}
	if exists {
		if err := s.authorizeTusOverride(w, r, target, r.URL.Query().Get("override") == "true"); err != nil {
			return
		}
		// Removing first keeps semantics explicit instead of relying on
		// O_TRUNC inside StartUpload.
		if err := s.store.Remove(r.Context(), target); err != nil {
			respondErr(w, err)
			return
		}
	} else if !s.canWrite(w, r, target) {
		return
	}

	up, err := s.store.StartUpload(r.Context(), target, length)
	if err != nil {
		respondErr(w, err)
		return
	}

	id, err := tusID()
	if err != nil {
		_ = up.Abort()
		apiError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.tus.add(&tusSession{id: id, path: target, size: length, up: up, lastUsed: time.Now()})

	h := w.Header()
	h.Set("Location", s.tusLocation(id))
	h.Set("Upload-Offset", "0")
	w.WriteHeader(http.StatusCreated)
}

// resolveTusTarget extracts the upload target and declared length from the
// request, replying with an error when either is missing or out of bounds.
func (s *Server) resolveTusTarget(w http.ResponseWriter, r *http.Request, meta map[string]string) (string, int64, bool) {
	target := r.URL.Query().Get("path")
	if target == "" {
		target = meta["path"]
	}
	if target == "" {
		apiError(w, http.StatusBadRequest, "missing upload target")
		return "", 0, false
	}
	if rejectReservedRoot(w, target) {
		return "", 0, false
	}

	length, err := strconv.ParseInt(r.Header.Get("Upload-Length"), 10, 64)
	if err != nil || length < 0 {
		apiError(w, http.StatusBadRequest, "missing or invalid Upload-Length")
		return "", 0, false
	}
	if length > s.tus.maxSize {
		apiError(w, http.StatusRequestEntityTooLarge, "upload exceeds maximum size")
		return "", 0, false
	}
	return target, length, true
}

// tusLocation returns the session URL, honoring a configured base path.
func (s *Server) tusLocation(id string) string {
	if s.cfg != nil && s.cfg.BaseURL != "" {
		return s.cfg.BaseURL + "/api/tus/" + id
	}
	return "/api/tus/" + id
}

// targetExists reports whether the upload target already exists.
func (s *Server) targetExists(ctx context.Context, path string) (bool, error) {
	_, err := s.store.Stat(ctx, path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// authorizeTusOverride gates replacing an existing file: signed-in writers
// with write permission on the target (admins anywhere, scoped users in
// their own scope). Anonymous guests get 403 and must pick a fresh name.
func (s *Server) authorizeTusOverride(w http.ResponseWriter, r *http.Request, target string, override bool) error {
	if !override {
		apiError(w, http.StatusConflict, "file already exists")
		return errTusHandled
	}
	if _, err := s.identity(r); err != nil {
		apiError(w, http.StatusForbidden, "overwriting requires an account")
		return errTusHandled
	}
	if !s.canWrite(w, r, target) {
		return errTusHandled
	}
	return nil
}

// errTusHandled marks an error whose HTTP response was already written.
var errTusHandled = errors.New("tus: error response already sent")

// handleTusDelete implements the tus termination extension: it aborts an
// in-flight upload and removes its partial file, so cancelling from the SPA
// never leaves a corrupt half-written target behind. Completing uploads race
// naturally: if the session is gone the partial became a full file and a 404
// is answered.
func (s *Server) handleTusDelete(w http.ResponseWriter, r *http.Request) {
	tusCommon(w)
	sess, ok := s.tus.get(r.PathValue("id"))
	if !ok {
		apiError(w, http.StatusNotFound, "unknown upload")
		return
	}
	s.tus.remove(sess.id)
	sess.mu.Lock()
	err := sess.up.Abort() // closes the handle and removes the partial
	sess.mu.Unlock()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		respondErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleTusHead reports the current offset so clients can resume.
func (s *Server) handleTusHead(w http.ResponseWriter, r *http.Request) {
	tusCommon(w)
	sess, ok := s.tus.get(r.PathValue("id"))
	if !ok {
		apiError(w, http.StatusNotFound, "unknown upload")
		return
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()

	offset, err := sess.up.Offset()
	if err != nil {
		respondErr(w, err)
		return
	}
	h := w.Header()
	h.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	h.Set("Upload-Length", strconv.FormatInt(sess.size, 10))
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

// handleTusPatch appends the request body at the declared offset. The
// authoritative offset is the on-disk size; a stale Upload-Offset yields 409.
func (s *Server) handleTusPatch(w http.ResponseWriter, r *http.Request) {
	tusCommon(w)
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/offset+octet-stream") {
		drainTusBody(r)
		apiError(w, http.StatusBadRequest, "unsupported media type")
		return
	}

	sess, ok := s.tus.get(r.PathValue("id"))
	if !ok {
		drainTusBody(r)
		apiError(w, http.StatusNotFound, "unknown upload")
		return
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()

	offset, err := sess.up.Offset()
	if err != nil {
		drainTusBody(r)
		respondErr(w, err)
		return
	}

	reqOffset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		drainTusBody(r)
		apiError(w, http.StatusBadRequest, "missing Upload-Offset")
		return
	}
	if reqOffset != offset {
		drainTusBody(r)
		apiError(w, http.StatusConflict, "offset mismatch")
		return
	}
	if offset >= sess.size {
		drainTusBody(r)
		apiError(w, http.StatusBadRequest, "upload already complete")
		return
	}

	remaining := sess.size - offset
	written, overflow, err := copyTusBody(w, r, sess.up, offset, remaining)
	if err != nil {
		// The client may still be mid-send; drain what is left so the
		// error response is not followed by a connection reset.
		drainTusBody(r)
		respondErr(w, err)
		return
	}
	if overflow {
		// The client sent more than it declared; keep the file at exactly
		// Upload-Length bytes and report success like filebrowser does.
		s.log.Warn("tus patch overflow clamped", "path", sess.path)
	}

	newOffset := offset + written
	h := w.Header()
	h.Set("Upload-Offset", strconv.FormatInt(newOffset, 10))
	if newOffset >= sess.size {
		// Completed: commit (fsync + close) and drop the session.
		if err := sess.up.Commit(); err != nil {
			respondErr(w, err)
			return
		}
		s.tus.remove(sess.id)
	}
	w.WriteHeader(http.StatusNoContent)
}

// copyTusBody streams the PATCH body into the upload at offset, clamping to
// the declared remaining bytes. Overflow (client bug) is reported, not fatal.
func copyTusBody(w http.ResponseWriter, r *http.Request, up storage.Upload, offset, remaining int64) (written int64, overflow bool, err error) {
	// Read one byte beyond remaining to detect overflow attempts.
	body := http.MaxBytesReader(w, r.Body, remaining+1)
	buf := make([]byte, min(tusChunkBuffer, remaining))

	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			clamped := int64(n)
			if written+clamped > remaining {
				clamped = remaining - written
				overflow = true
			}
			if _, werr := up.WriteAt(buf[:clamped], offset+written); werr != nil {
				return written, overflow, werr
			}
			written += clamped
		}
		if rerr == io.EOF || overflow {
			break
		}
		if rerr != nil {
			return written, overflow, rerr
		}
		if written >= remaining {
			break
		}
	}
	return written, overflow, nil
}

// parseTusMetadata decodes the Upload-Metadata header: comma-separated
// "key base64value" pairs.
func parseTusMetadata(header string) map[string]string {
	out := map[string]string{}
	for pair := range strings.SplitSeq(header, ",") {
		parts := strings.Fields(strings.TrimSpace(pair))
		if len(parts) == 0 {
			continue
		}
		key := parts[0]
		if len(parts) == 2 {
			if decoded, err := base64.StdEncoding.DecodeString(parts[1]); err == nil {
				out[key] = string(decoded)
				continue
			}
		}
		out[key] = ""
	}
	return out
}
