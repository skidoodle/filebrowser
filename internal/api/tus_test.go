package api

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/skidoodle/filebrowser/internal/storage"
)

func newTestServer(t *testing.T, maxUpload int64) (*httptest.Server, storage.Storage) {
	t.Helper()
	ts, store, _ := newInsecureTestServer(t, maxUpload)
	return ts, store
}

// tusCreate starts an upload and returns the upload URL.
func tusCreate(t *testing.T, ts *httptest.Server, path string, length int64, extraQuery string) string {
	t.Helper()
	url := ts.URL + "/api/tus?path=" + path + extraQuery
	req := newTestReq(t, http.MethodPost, url, nil)
	req.Header.Set("Upload-Length", strconv.FormatInt(length, 10))
	res := do(t, req)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("create status = %d, want 201 (body: %s)", res.StatusCode, body)
	}
	loc := res.Header.Get("Location")
	if loc == "" {
		t.Fatal("missing Location header")
	}
	if got := res.Header.Get("Tus-Resumable"); got != "1.0" {
		t.Fatalf("Tus-Resumable = %q", got)
	}
	return ts.URL + loc
}

func tusPatch(t *testing.T, url string, data []byte, offset int64, wantStatus int) {
	t.Helper()
	req := newTestReq(t, http.MethodPatch, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	req.Header.Set("Upload-Offset", strconv.FormatInt(offset, 10))
	res := do(t, req)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != wantStatus {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("patch status = %d, want %d (body: %s)", res.StatusCode, wantStatus, body)
	}
}

func tusOffset(t *testing.T, url string) int64 {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodHead, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("head status = %d", res.StatusCode)
	}
	n, err := strconv.ParseInt(res.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		t.Fatalf("bad Upload-Offset header: %v", err)
	}
	return n
}

func TestTusUploadLifecycle(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)

	url := tusCreate(t, ts, "videos/movie.bin", 10, "")
	if got := tusOffset(t, url); got != 0 {
		t.Fatalf("initial offset = %d, want 0", got)
	}

	// Offset mismatch must be rejected.
	tusPatch(t, url, []byte("0123456789"), 4, http.StatusConflict)

	// Upload in two chunks.
	tusPatch(t, url, []byte("01234"), 0, http.StatusNoContent)
	if got := tusOffset(t, url); got != 5 {
		t.Fatalf("mid offset = %d, want 5", got)
	}
	tusPatch(t, url, []byte("56789"), 5, http.StatusNoContent)

	// Verify file content.
	rc, info, err := store.Open(t.Context(), "videos/movie.bin")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "0123456789" || info.Size != 10 {
		t.Fatalf("content = %q size = %d", got, info.Size)
	}

	// Patching a completed upload fails: the session is dropped on
	// completion, so the URL is gone.
	tusPatch(t, url, []byte("x"), 10, http.StatusNotFound)
}

func TestTusConflictAndLimits(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 8)

	// Over max size → 413.
	req := newTestReq(t, http.MethodPost, ts.URL+"/api/tus?path=big.bin", nil)
	req.Header.Set("Upload-Length", "9")
	res := do(t, req)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status = %d, want 413", res.StatusCode)
	}

	// Create existing file without override → 409.
	if _, err := store.CreateFile(t.Context(), "exists.txt"); err != nil {
		t.Fatal(err)
	}
	req2 := newTestReq(t, http.MethodPost, ts.URL+"/api/tus?path=exists.txt", nil)
	req2.Header.Set("Upload-Length", "1")
	res2 := do(t, req2)
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusConflict {
		t.Fatalf("conflict status = %d, want 409", res2.StatusCode)
	}

	// With override → 201, old content replaced (the guest-403 case is
	// covered by TestTusOverrideGatedBehindAdmin; this server runs in
	// insecure mode).
	url := tusCreate(t, ts, "exists.txt", 3, "&override=true")
	tusPatch(t, url, []byte("abc"), 0, http.StatusNoContent)
	rc, _, err := store.Open(t.Context(), "exists.txt")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(content) != "abc" {
		t.Fatalf("after override content = %q", content)
	}
}

func TestTusOverflowClamped(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	url := tusCreate(t, ts, "clamp.bin", 4, "")
	tusPatch(t, url, []byte("abcdefgh"), 0, http.StatusNoContent)
	rc, info, err := store.Open(t.Context(), "clamp.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if info.Size != 4 {
		t.Fatalf("size = %d, want 4 (clamped)", info.Size)
	}
}

// TestTusPatchRejectDrainsBody locks in the contract around rejected PATCH
// requests carrying a full-size chunk: the client gets a real status code
// (not a connection reset, which browsers surface as ERR_CONNECTION_RESET
// and tus clients cannot act on) and the session stays usable so the upload
// can complete afterwards. The reject paths read the body before responding
// so the Go http server never closes the connection with unread data.
func TestTusPatchRejectDrainsBody(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 64<<20)
	url := tusCreate(t, ts, "drain.bin", 20<<20, "")

	// A stale-offset PATCH carrying a full default chunk (16 MiB): the
	// server must consume the body before responding 409.
	big := make([]byte, 16<<20)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPatch, url, bytes.NewReader(big))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	req.Header.Set("Upload-Offset", "1048576") // wrong offset → 409
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("rejected PATCH did not receive a response: %v", err)
	}
	if res.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		t.Fatalf("patch status = %d, want 409 (body: %s)", res.StatusCode, body)
	}
	_ = res.Body.Close()

	// The session survived the rejection: finish the upload with correct
	// offsets and verify the content (also releases the file handle on
	// Windows so TempDir cleanup succeeds).
	tusPatch(t, url, big, 0, http.StatusNoContent)
	rest := make([]byte, 4<<20)
	tusPatch(t, url, rest, 16<<20, http.StatusNoContent)

	rc, info, err := store.Open(t.Context(), "drain.bin")
	if err != nil {
		t.Fatal(err)
	}
	size, err := io.Copy(io.Discard, rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != 20<<20 || size != 20<<20 {
		t.Fatalf("size = %d/%d, want %d", info.Size, size, 20<<20)
	}
}

// TestTusTerminate locks in the termination extension: cancelling an
// in-flight upload must remove the partial file, so a cancelled video does
// not linger as a corrupt half-written target blocking re-upload.
func TestTusTerminate(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)

	url := tusCreate(t, ts, "videos/cancelled.bin", 10, "")
	tusPatch(t, url, []byte("01234"), 0, http.StatusNoContent)
	if _, err := store.Stat(t.Context(), "videos/cancelled.bin"); err != nil {
		t.Fatalf("partial should exist while uploading: %v", err)
	}

	// Terminate.
	req := newTestReq(t, http.MethodDelete, url, nil)
	res := do(t, req)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("delete status = %d, want 204 (body: %s)", res.StatusCode, body)
	}

	// The partial and the session are gone.
	if _, err := store.Stat(t.Context(), "videos/cancelled.bin"); err == nil {
		t.Fatal("partial still exists after termination")
	}
	req2, err := http.NewRequestWithContext(t.Context(), http.MethodHead, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("head after termination status = %d, want 404", res2.StatusCode)
	}

	// Terminating again answers 404.
	req3 := newTestReq(t, http.MethodDelete, url, nil)
	res3 := do(t, req3)
	defer func() { _ = res3.Body.Close() }()
	if res3.StatusCode != http.StatusNotFound {
		t.Fatalf("repeat delete status = %d, want 404", res3.StatusCode)
	}
}

func TestParseTusMetadata(t *testing.T) {
	t.Parallel()
	header := "filename " + base64Of("report.pdf") + ",empty,relp " + base64Of("docs/a b.txt")
	meta := parseTusMetadata(header)
	if meta["filename"] != "report.pdf" || meta["empty"] != "" || meta["relp"] != "docs/a b.txt" {
		t.Fatalf("parsed: %#v", meta)
	}
}

func base64Of(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
