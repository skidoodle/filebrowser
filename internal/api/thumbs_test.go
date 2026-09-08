package api

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skidoodle/filebrowser/internal/storage/local"
)

func writeTestImage(t *testing.T, s *local.Storage, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	up, err := s.StartUpload(t.Context(), path, 0)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if _, err := up.WriteAt(buf.Bytes(), 0); err != nil {
		t.Fatal(err)
	}
	if err := up.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestThumbGenerationAndCache(t *testing.T) {
	t.Parallel()
	ts, store, cfg := newInsecureTestServer(t, 1<<20)
	ls, ok := store.(*local.Storage)
	if !ok {
		t.Fatal("unexpected storage type")
	}
	writeTestImage(t, ls, "pics/photo.png", 800, 600)

	res := testGet(t, ts, ts.URL+"/api/thumb?path=pics/photo.png")
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "image/jpeg") {
		t.Fatalf("content type = %q", res.Header.Get("Content-Type"))
	}
	if len(body) < 100 || body[0] != 0xFF || body[1] != 0xD8 {
		t.Fatalf("not a JPEG payload (%d bytes)", len(body))
	}

	// A cached file must exist in the configured cache dir.
	matches, err := filepath.Glob(filepath.Join(cfg.CacheDir, "previews", "*.jpg"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no cache files: %v %v", matches, err)
	}

	// Second request must be served (from cache) with identical bytes.
	res2 := testGet(t, ts, ts.URL+"/api/thumb?path=pics/photo.png")
	body2, _ := io.ReadAll(res2.Body)
	_ = res2.Body.Close()
	if !bytes.Equal(body, body2) {
		t.Fatal("cached response differs")
	}
}

func TestThumbRejectsNonImage(t *testing.T) {
	t.Parallel()
	ts, store, _ := newInsecureTestServer(t, 1<<20)
	ls, ok := store.(*local.Storage)
	if !ok {
		t.Fatal("unexpected storage type")
	}
	writeTestImage(t, ls, "pics/photo.png", 64, 64)

	// Directory → 400.
	res := testGet(t, ts, ts.URL+"/api/thumb?path=pics")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("dir status = %d, want 400", res.StatusCode)
	}
}

func TestBigPreview(t *testing.T) {
	t.Parallel()
	ts, store, _ := newInsecureTestServer(t, 1<<20)
	ls, ok := store.(*local.Storage)
	if !ok {
		t.Fatal("unexpected storage type")
	}
	writeTestImage(t, ls, "pics/big.png", 4000, 3000)

	res := testGet(t, ts, ts.URL+"/api/big?path=pics/big.png")
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	// Fit within 1080.
	cfg, _, err := image.DecodeConfig(res.Body)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Width > 1080 || cfg.Height > 1080 {
		t.Fatalf("preview not fitted: %dx%d", cfg.Width, cfg.Height)
	}
}

func TestBigPassthroughForGifAndSvg(t *testing.T) {
	t.Parallel()
	ts, store, _ := newInsecureTestServer(t, 1<<20)
	ls, ok := store.(*local.Storage)
	if !ok {
		t.Fatal("unexpected storage type")
	}

	// Minimal valid GIF (1x1) written through the upload port.
	up, err := ls.StartUpload(t.Context(), "pics/anim.gif", 0)
	if err != nil {
		t.Fatal(err)
	}
	gifBytes := []byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00,
		0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x2c, 0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
	}
	if _, err := up.WriteAt(gifBytes, 0); err != nil {
		t.Fatal(err)
	}
	if err := up.Commit(); err != nil {
		t.Fatal(err)
	}

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"/>`)
	up, err = ls.StartUpload(t.Context(), "pics/icon.svg", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := up.WriteAt(svg, 0); err != nil {
		t.Fatal(err)
	}
	if err := up.Commit(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path, contentType string
		want              []byte
	}{
		{"pics/anim.gif", "image/gif", gifBytes},
		{"pics/icon.svg", "image/svg+xml", svg},
	} {
		res := testGet(t, ts, ts.URL+"/api/big?path="+tc.path)
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: status = %d", tc.path, res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, tc.contentType) {
			t.Fatalf("%s: content type = %q", tc.path, ct)
		}
		if res.Header.Get("Content-Security-Policy") == "" {
			t.Fatalf("%s: missing CSP sandbox header", tc.path)
		}
		if !bytes.Equal(body, tc.want) {
			t.Fatalf("%s: bytes not passed through", tc.path)
		}
	}
}

func TestOrientationAndTransforms(t *testing.T) {
	t.Parallel()
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	src.Set(0, 0, color.RGBA{R: 255, A: 255})

	for orient := 1; orient <= 8; orient++ {
		res := applyOrientation(src, orient)
		if res == nil {
			t.Fatalf("orient %d returned nil", orient)
		}
	}

	fitted := fitImage(src, 20)
	if fitted.Bounds().Dx() > 20 || fitted.Bounds().Dy() > 20 {
		t.Fatalf("fitted bounds too large: %v", fitted.Bounds())
	}

	thumb := cropSquareThumbnail(src, 32)
	if thumb.Bounds().Dx() != 32 || thumb.Bounds().Dy() != 32 {
		t.Fatalf("thumb bounds not 32x32: %v", thumb.Bounds())
	}
}
