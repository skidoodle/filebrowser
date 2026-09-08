package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/skidoodle/filebrowser/internal/storage"
)

// handleSubtitle serves a subtitle sidecar for a video as WebVTT.
// It looks for sibling files matching the video stem (optionally with a
// language tag) in .vtt or .srt form and converts SRT to VTT on the fly.
// Query: path (the video file).
func (s *Server) handleSubtitle(w http.ResponseWriter, r *http.Request) {
	videoPath := r.URL.Query().Get("path")
	if !s.canRead(w, r, videoPath) {
		return
	}
	info, err := s.store.Stat(r.Context(), videoPath)
	if err != nil {
		respondErr(w, err)
		return
	}
	if info.IsDir || info.Type != storage.TypeVideo {
		apiError(w, http.StatusBadRequest, "not a video file")
		return
	}

	sub, err := s.findSubtitle(r, videoPath, info.Name)
	if err != nil {
		if errors.Is(err, errSubtitleNotFound) {
			// No sidecar: serve an empty track so players can attach it
			// unconditionally without console noise from failed probes.
			w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
			_, _ = w.Write([]byte("WEBVTT\n\n"))
			return
		}
		respondErr(w, err)
		return
	}

	rc, _, err := s.store.Open(r.Context(), sub.path)
	if err != nil {
		respondErr(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if sub.isVTT {
		_, _ = io.Copy(w, rc)
		return
	}
	body, err := io.ReadAll(rc)
	if err != nil {
		return // headers already sent; truncating is the best we can do
	}
	_, _ = w.Write([]byte(srtToVTT(string(body))))
}

type subtitleRef struct {
	path  string
	isVTT bool
}

// findSubtitle locates a subtitle sidecar next to the video.
func (s *Server) findSubtitle(r *http.Request, videoPath, videoName string) (subtitleRef, error) {
	dir := parentOf(videoPath)
	stem := strings.TrimSuffix(videoName, "."+infoExtension(videoName))

	listing, err := s.store.List(r.Context(), dir, storage.SortOptions{})
	if err != nil {
		return subtitleRef{}, err
	}

	sub, ok := matchSubtitle(listing.Items, stem)
	if !ok {
		return subtitleRef{}, errSubtitleNotFound
	}
	return sub, nil
}

// matchSubtitle picks the best sidecar: exact stem.vtt > stem.srt >
// language-tagged variants (movie.en.vtt / movie.de.srt).
func matchSubtitle(items []storage.FileInfo, stem string) (subtitleRef, bool) {
	var fallback subtitleRef
	has := false
	for _, item := range items {
		if item.IsDir {
			continue
		}
		switch {
		case item.Name == stem+".vtt":
			return subtitleRef{path: item.Path, isVTT: true}, true
		case item.Name == stem+".srt":
			return subtitleRef{path: item.Path, isVTT: false}, true
		case strings.HasPrefix(item.Name, stem+".") && strings.HasSuffix(item.Name, ".vtt"):
			// Prefer a vtt fallback over an srt one; exact match preferred.
			if !has || fallback.isVTT {
				fallback = subtitleRef{path: item.Path, isVTT: true}
				has = true
			}
		case strings.HasPrefix(item.Name, stem+".") && strings.HasSuffix(item.Name, ".srt"):
			if !has {
				fallback = subtitleRef{path: item.Path, isVTT: false}
				has = true
			}
		}
	}
	return fallback, has
}

var errSubtitleNotFound = apiErrorf(http.StatusNotFound, "no subtitle found")

func infoExtension(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return ""
}

func parentOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return "."
}

// srtToVTT converts SubRip text to WebVTT: header line, timing commas to
// dots, and stripped cue numbering are all a browser needs.
func srtToVTT(srt string) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")

	for block := range strings.SplitSeq(strings.ReplaceAll(srt, "\r\n", "\n"), "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		// Drop a leading numeric cue index.
		if len(lines) > 1 && isCueIndex(lines[0]) {
			lines = lines[1:]
		}
		if len(lines) == 0 || !strings.Contains(lines[0], "-->") {
			continue
		}
		lines[0] = strings.ReplaceAll(lines[0], ",", ".")
		for _, line := range lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func isCueIndex(line string) bool {
	for _, r := range line {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(line) > 0
}

// apiErrorf builds a sentinel-ish error carrying an HTTP status for respondErr.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

func apiErrorf(status int, format string, args ...any) error {
	return &httpError{status: status, msg: fmt.Sprintf(format, args...)}
}
