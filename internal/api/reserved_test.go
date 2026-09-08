package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestReservedRootNamesRejected locks in the route/name collision guard:
// root-level entries named after SPA routes could never be browsed, so
// writes whose first segment is reserved are refused on every mutating
// endpoint. Nested reserved names ("docs/view") stay legal.
func TestReservedRootNamesRejected(t *testing.T) {
	t.Parallel()

	ts, _, _ := newInsecureTestServer(t, 1<<20)

	for _, path := range []string{"view", "new", "settings", "login", "setup"} {
		t.Run("dir/"+path, func(t *testing.T) {
			t.Parallel()

			res := postJSON(t, ts, epDir, `{"path":"`+path+`"}`, nil)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("create dir %q status = %d, want 400", path, res.StatusCode)
			}
		})
	}

	for _, path := range []string{"view/notes.txt", "settings/sub/deep.txt"} {
		t.Run("file/"+path, func(t *testing.T) {
			t.Parallel()

			res := postJSON(t, ts, "/api/file", `{"path":"`+path+`"}`, nil)
			_ = res.Body.Close()
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("create file %q status = %d, want 400", path, res.StatusCode)
			}
		})
	}

	t.Run("save under reserved root", func(t *testing.T) {
		t.Parallel()

		req := newTestReq(t, http.MethodPut, ts.URL+"/api/raw?path=login/notes.txt", strings.NewReader("hello"))
		req.Header.Set(hdrOrigin, ts.URL)
		res := do(t, req)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("save status = %d, want 400", res.StatusCode)
		}
	})

	t.Run("upload under reserved root", func(t *testing.T) {
		t.Parallel()

		req := newTestReq(t, http.MethodPost, ts.URL+"/api/tus?path=setup/clip.bin", nil)
		req.Header.Set(hdrOrigin, ts.URL)
		req.Header.Set("Upload-Length", "4")
		res := do(t, req)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("tus create status = %d, want 400", res.StatusCode)
		}
	})

	t.Run("move onto reserved root", func(t *testing.T) {
		t.Parallel()

		res := postJSON(t, ts, epDir, `{"path":"docs"}`, nil)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create docs status = %d, want 201", res.StatusCode)
		}

		res = postJSON(t, ts, epMove, `{"from":"docs","to":"new"}`, nil)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("move status = %d, want 400", res.StatusCode)
		}
	})

	t.Run("nested reserved names are allowed", func(t *testing.T) {
		t.Parallel()

		res := postJSON(t, ts, epDir, `{"path":"docs/view"}`, nil)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create docs/view status = %d, want 201", res.StatusCode)
		}
	})
}
