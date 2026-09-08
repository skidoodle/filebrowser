package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skidoodle/filebrowser/internal/config"
	"github.com/skidoodle/filebrowser/internal/storage"
)

// newInsecureTestServer builds a fully-featured server in insecure mode
// (every visitor is admin), matching the old helper signature.
func newInsecureTestServer(t *testing.T, maxUpload int64) (*httptest.Server, storage.Storage, *config.Config) {
	t.Helper()
	return newTestServerCfg(t, testServerConfig{Insecure: true, MaxUpload: maxUpload})
}

// testGet performs a context-aware GET against the test server, satisfying
// noctx, and returns the response (caller closes the body).
func testGet(t *testing.T, _ *httptest.Server, url string) *http.Response {
	t.Helper()
	return do(t, newTestReq(t, http.MethodGet, url, nil))
}

// newTestReq builds a context-aware request for the test server.
func newTestReq(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// do sends a request, failing the test on transport errors.
func do(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

var _ = io.Discard
