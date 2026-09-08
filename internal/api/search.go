package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/skidoodle/filebrowser/internal/storage"
)

// handleSearch streams newline-delimited JSON results while keeping the
// connection alive with heartbeat pings, so reverse proxies and browsers
// tolerate long walks over large trees.
// Query: q (substring), type, ext, limit, path (walk root).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	opts := searchOptions(r.URL.Query())

	flusher, ok := w.(http.Flusher)
	if !ok {
		apiError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	searchHeaders(w.Header())
	w.WriteHeader(http.StatusOK)

	var mu sync.Mutex
	write := func(b []byte) bool {
		mu.Lock()
		defer mu.Unlock()
		if _, err := w.Write(b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	q := r.URL.Query()
	ctx := r.Context()
	done := make(chan struct{})
	go heartbeat(ctx, done, write)

	err := s.store.Walk(ctx, q.Get("path"), opts, func(fi storage.FileInfo) error {
		// Private folders of others must not leak via search matches —
		// including anything nested below them.
		if s.authz != nil && !s.cfg.Insecure && s.authz.CanRead(s.readerActor(r), fi.Path) != nil {
			return nil //nolint:nilerr // skipping hidden entries is the point
		}
		line, err := json.Marshal(fi)
		if err != nil {
			return err
		}
		line = append(line, '\n')
		mu.Lock()
		_, werr := w.Write(line)
		flusher.Flush()
		mu.Unlock()
		if werr != nil {
			return context.Canceled
		}
		return nil
	})

	close(done)
	term := q.Get("q")
	switch {
	case err == nil, errors.Is(err, context.Canceled), errors.Is(err, storage.ErrDone):
		// client went away or search finished
	case errors.Is(err, context.DeadlineExceeded):
		s.log.Warn("search timed out", "q", term)
	default:
		// Headers are already sent; a JSON error is impossible, so log only.
		s.log.Error("search failed", "err", err)
	}
}

// searchOptions parses the search query parameters.
func searchOptions(q url.Values) storage.SearchOptions {
	return storage.SearchOptions{
		Term:      q.Get("q"),
		Type:      storage.FileType(q.Get("type")),
		Extension: strings.TrimPrefix(strings.ToLower(q.Get("ext")), "."),
		Limit:     parseLimit(q.Get("limit"), 1000, 10000),
	}
}

// searchHeaders sets the NDJSON streaming headers.
func searchHeaders(h http.Header) {
	h.Set("Content-Type", "application/x-ndjson; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
}

// heartbeat keeps the connection alive while the walk runs.
func heartbeat(ctx context.Context, done <-chan struct{}, write func([]byte) bool) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			if !write([]byte("\n")) {
				return
			}
		}
	}
}
