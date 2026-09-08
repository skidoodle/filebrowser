package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/skidoodle/filebrowser/internal/storage"
)

func TestSrtToVTT(t *testing.T) {
	t.Parallel()
	srt := "1\r\n00:00:01,000 --> 00:00:04,500\r\nHello there\r\n\r\n2\r\n00:01:02,250 --> 00:01:05,000\r\nSecond line\r\nmulti\r\n"

	vtt := srtToVTT(srt)
	if !strings.HasPrefix(vtt, "WEBVTT\n\n") {
		t.Fatalf("missing WEBVTT header: %q", vtt)
	}
	if strings.Contains(vtt, ",") {
		t.Fatalf("commas not converted: %q", vtt)
	}
	if strings.Contains(vtt, "00:00:01.000 --> 00:00:04.500") == false {
		t.Fatalf("timing mangled: %q", vtt)
	}
	if strings.Contains(vtt, "\n1\n") || strings.Contains(vtt, "\n2\n") {
		t.Fatalf("cue indexes not stripped: %q", vtt)
	}
}

func TestSubtitleEndpoint(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	ctx := t.Context()

	// Create a video and an SRT sidecar.
	if _, err := store.CreateFile(ctx, "movies/movie.mp4"); err != nil {
		t.Fatal(err)
	}
	srt, err := store.CreateFile(ctx, "movies/movie.srt")
	if err != nil {
		t.Fatal(err)
	}
	_ = srt
	up, err := store.StartUpload(ctx, "movies/movie.srt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := up.WriteAt([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"), 0); err != nil {
		t.Fatal(err)
	}
	if err := up.Commit(); err != nil {
		t.Fatal(err)
	}

	// stat must classify the empty file as video via extension
	info, err := store.Stat(ctx, "movies/movie.mp4")
	if err != nil || info.Type != storage.TypeVideo {
		t.Fatalf("video stat: %v %+v", err, info)
	}

	res := testGet(t, ts, ts.URL+"/api/subtitle?path=movies/movie.mp4")
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/vtt") {
		t.Fatalf("content type = %q", ct)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.HasPrefix(string(body), "WEBVTT") || !strings.Contains(string(body), "00:00:01.000") {
		t.Fatalf("vtt body: %q", body)
	}
}

func TestSubtitleMissingSidecar(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	if _, err := store.CreateFile(t.Context(), "movies/none.mp4"); err != nil {
		t.Fatal(err)
	}
	res := testGet(t, ts, ts.URL+"/api/subtitle?path=movies/none.mp4")
	defer func() { _ = res.Body.Close() }()
	// No sidecar still yields an empty, valid VTT (players attach it
	// unconditionally; 404 probes would spam the console).
	if res.StatusCode != http.StatusOK {
		t.Fatalf("no-sidecar status = %d, want 200", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if string(body) != "WEBVTT\n\n" {
		t.Fatalf("empty vtt body = %q", body)
	}
}
