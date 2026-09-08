package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/skidoodle/filebrowser/internal/storage"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// decodeSem bounds concurrent image decodes; large originals are CPU/RAM hungry.
var decodeSem = make(chan struct{}, 2)

const (
	thumbSize = 256
	bigSize   = 1080
	jpegOpts  = 82
)

// handleThumb serves a small square crop for mosaic/gallery grids.
func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	s.servePreview(w, r, thumbSize, true, false)
}

// handleBig serves a 1080px-fit preview for lightbox viewing.
func (s *Server) handleBig(w http.ResponseWriter, r *http.Request) {
	s.servePreview(w, r, bigSize, false, true)
}

// passthroughExt lists image formats the preview pipeline must not
// re-encode: svg is not rasterizable here (500 without the fallback),
// and re-encoding an animated gif destroys every frame but the first.
var passthroughExt = map[string]bool{
	"gif": true,
	"svg": true,
}

// servePreview generates (or serves from the disposable disk cache) a
// resized JPEG of an image. Cache keys include modtime+size so edits
// invalidate automatically. With rawOK, formats listed in passthroughExt
// stream untouched instead.
func (s *Server) servePreview(w http.ResponseWriter, r *http.Request, maxDim int, crop, rawOK bool) {
	if !s.canRead(w, r, r.URL.Query().Get("path")) {
		return
	}
	info, err := s.store.Stat(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		respondErr(w, err)
		return
	}
	if info.IsDir || info.Type != storage.TypeImage {
		apiError(w, http.StatusBadRequest, "not an image")
		return
	}
	// Refuse to decode absurd payloads into memory.
	if info.Size > 512<<20 {
		apiError(w, http.StatusRequestEntityTooLarge, "image too large to preview")
		return
	}

	if rawOK && passthroughExt[strings.ToLower(info.Extension)] {
		s.serveOriginal(w, r, info)
		return
	}

	cachePath, err := s.previewCachePath(info.Path, info.ModTime.Unix(), info.Size, maxDim, crop)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := os.Stat(cachePath); err != nil {
		if err := s.renderPreview(r, info.Path, cachePath, maxDim, crop); err != nil {
			// Unrasterizable or corrupt image: stream the original bytes
			// rather than failing — the browser may still render it.
			s.serveOriginal(w, r, info)
			return
		}
	}

	// cachePath is a sha256-derived name inside our own cache dir.
	f, err := os.Open(cachePath)
	if err != nil {
		respondErr(w, err)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		respondErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, "preview.jpg", fi.ModTime(), f)
}

// serveOriginal streams the untouched file with the same sandbox headers
// as the raw endpoint, since this can serve vector formats directly.
func (s *Server) serveOriginal(w http.ResponseWriter, r *http.Request, info storage.FileInfo) {
	rc, _, err := s.store.Open(r.Context(), info.Path)
	if err != nil {
		respondErr(w, err)
		return
	}
	defer rc.Close()

	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Content-Type", info.MimeType)
	if cd := mime.FormatMediaType("inline", map[string]string{"filename": info.Name}); cd != "" {
		h.Set("Content-Disposition", cd)
	}
	h.Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, info.Name, info.ModTime, rc)
}

// renderPreview decodes, resizes and encodes into the cache file.
func (s *Server) renderPreview(r *http.Request, path, cachePath string, maxDim int, crop bool) error {
	decodeSem <- struct{}{}
	defer func() { <-decodeSem }()

	rc, _, err := s.store.Open(r.Context(), path)
	if err != nil {
		return err
	}
	defer rc.Close()

	src, orientation, err := decodeImage(rc)
	if err != nil {
		return fmt.Errorf("preview: decode: %w", err)
	}
	if orientation > 1 {
		src = applyOrientation(src, orientation)
	}

	var out image.Image
	if crop {
		out = cropSquareThumbnail(src, maxDim)
	} else {
		out = fitImage(src, maxDim)
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return err
	}
	tmp := cachePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := jpeg.Encode(f, out, &jpeg.Options{Quality: jpegOpts}); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, cachePath)
}

// cropSquareThumbnail crops the center square of src and resizes it to size x size.
func cropSquareThumbnail(src image.Image, size int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	var cropRect image.Rectangle
	if w > h {
		offset := (w - h) / 2
		cropRect = image.Rect(b.Min.X+offset, b.Min.Y, b.Min.X+offset+h, b.Max.Y)
	} else {
		offset := (h - w) / 2
		cropRect = image.Rect(b.Min.X, b.Min.Y+offset, b.Max.X, b.Min.Y+offset+w)
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, cropRect, draw.Over, nil)
	return dst
}

// fitImage scales src down so its longest edge is at most maxDim while preserving aspect ratio.
func fitImage(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if w <= maxDim && h <= maxDim {
		return src
	}
	scale := min(float64(maxDim)/float64(w), float64(maxDim)/float64(h))
	newW := max(1, int(float64(w)*scale))
	newH := max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// decodeImage reads EXIF orientation and decodes the image stream.
func decodeImage(r io.Reader) (image.Image, int, error) {
	buf := make([]byte, 64*1024)
	n, _ := io.ReadFull(r, buf)
	prefix := buf[:n]
	orientation := parseEXIFOrientation(prefix)
	fullReader := io.MultiReader(bytes.NewReader(prefix), r)
	img, _, err := image.Decode(fullReader)
	return img, orientation, err
}

// parseEXIFOrientation inspects JPEG header segments for EXIF orientation (tag 0x0112).
func parseEXIFOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	idx := 2
	for idx+4 <= len(b) {
		if b[idx] != 0xFF {
			return 1
		}
		marker := b[idx+1]
		idx += 2
		if marker == 0xDA || marker == 0xD9 {
			return 1
		}
		if idx+2 > len(b) {
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(b[idx : idx+2]))
		if segLen < 2 || idx+segLen > len(b) {
			return 1
		}
		if marker == 0xE1 {
			if orient := parseAPP1Exif(b[idx+2 : idx+segLen]); orient > 1 {
				return orient
			}
		}
		idx += segLen
	}
	return 1
}

func parseAPP1Exif(payload []byte) int {
	if len(payload) >= 14 && string(payload[:6]) == "Exif\x00\x00" {
		return parseTIFFOrientation(payload[6:])
	}
	return 1
}

// parseTIFFOrientation parses the orientation value from TIFF IFD0 entries.
func parseTIFFOrientation(b []byte) int {
	bo := tiffByteOrder(b)
	if bo == nil || len(b) < 8 || bo.Uint16(b[2:4]) != 42 {
		return 1
	}
	ifdOffset := int(bo.Uint32(b[4:8]))
	if ifdOffset < 8 || ifdOffset+2 > len(b) {
		return 1
	}
	numEntries := int(bo.Uint16(b[ifdOffset : ifdOffset+2]))
	return findOrientationTag(b, ifdOffset+2, numEntries, bo)
}

func tiffByteOrder(b []byte) binary.ByteOrder {
	if len(b) < 2 {
		return nil
	}
	switch {
	case b[0] == 'I' && b[1] == 'I':
		return binary.LittleEndian
	case b[0] == 'M' && b[1] == 'M':
		return binary.BigEndian
	default:
		return nil
	}
}

func findOrientationTag(b []byte, entryStart, numEntries int, bo binary.ByteOrder) int {
	for i := range numEntries {
		offset := entryStart + i*12
		if offset+12 > len(b) {
			return 1
		}
		if bo.Uint16(b[offset:offset+2]) == 0x0112 {
			val := int(bo.Uint16(b[offset+8 : offset+10]))
			if val >= 1 && val <= 8 {
				return val
			}
			return 1
		}
	}
	return 1
}

func flipH(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			dst.Set(w-1-x, y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func flipV(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			dst.Set(x, h-1-y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate90(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := range h {
		for x := range w {
			dst.Set(h-1-y, x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate180(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			dst.Set(w-1-x, h-1-y, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate270(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := range h {
		for x := range w {
			dst.Set(y, w-1-x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func transpose(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := range h {
		for x := range w {
			dst.Set(y, x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func transverse(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := range h {
		for x := range w {
			dst.Set(h-1-y, w-1-x, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

// applyOrientation rotates or flips img according to EXIF orientation tag (1-8).
func applyOrientation(img image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return flipH(img)
	case 3:
		return rotate180(img)
	case 4:
		return flipV(img)
	case 5:
		return transpose(img)
	case 6:
		return rotate90(img)
	case 7:
		return transverse(img)
	case 8:
		return rotate270(img)
	default:
		return img
	}
}

// previewCachePath builds a stable cache key for a preview request.
func (s *Server) previewCachePath(p string, modUnix, size int64, maxDim int, crop bool) (string, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d|%d|%t", p, modUnix, size, maxDim, crop)))
	dir := s.cfg.CacheDir
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "previews", hex.EncodeToString(sum[:])+".jpg"), nil
}
