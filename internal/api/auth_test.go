package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skidoodle/filebrowser/internal/auth"
	"github.com/skidoodle/filebrowser/internal/authz"
	"github.com/skidoodle/filebrowser/internal/config"
	"github.com/skidoodle/filebrowser/internal/storage"
	"github.com/skidoodle/filebrowser/internal/storage/local"
	"github.com/skidoodle/filebrowser/internal/store"
)

// newAuthTestServer builds a server with an auth manager rooted at a temp
// dir. seed optionally seeds the admin password via env semantics.
func newAuthTestServer(t *testing.T, seed, insecure string) (*httptest.Server, *config.Config) {
	t.Helper()
	ts, _, cfg := newTestServerCfg(t, testServerConfig{Seed: seed, Insecure: insecure == "true"})
	return ts, cfg
}

// testServerConfig describes a test server beyond the defaults.
type testServerConfig struct {
	Seed      string
	Insecure  bool
	MaxUpload int64
	// Users are created after setup: name, password, admin, scope.
	Users []testUser
}

type testUser struct {
	Username string
	Password string
	Admin    bool
	Scope    string
}

// newTestServerCfg builds a server with SQLite-backed accounts and the
// permission engine wired in.
func newTestServerCfg(t *testing.T, tc testServerConfig) (*httptest.Server, storage.Storage, *config.Config) {
	t.Helper()
	maxUpload := tc.MaxUpload
	if maxUpload == 0 {
		maxUpload = 1 << 20
	}
	cfg := &config.Config{
		Root:      t.TempDir(),
		Address:   "127.0.0.1:0",
		MaxUpload: maxUpload,
		CacheDir:  t.TempDir(),
		Insecure:  tc.Insecure,
	}
	log := slog.New(slog.DiscardHandler)
	fsStore, err := local.New(cfg.Root)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(cfg.Root, ".filebrowser", "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var mgr *auth.Manager
	if !cfg.Insecure {
		mgr, err = auth.New(filepath.Join(cfg.Root, ".filebrowser"), cfg.Secret, tc.Seed, st, log)
		if err != nil {
			t.Fatal(err)
		}
	}
	eng, err := authz.New(st)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{Config: cfg, Store: fsStore, Log: log, Version: "test", Auth: mgr, Authz: eng})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range tc.Users {
		if _, err := mgr.CreateUser(u.Username, u.Password, u.Admin, u.Scope); err != nil {
			t.Fatalf("create test user %s: %v", u.Username, err)
		}
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, fsStore, cfg
}

func postJSON(t *testing.T, ts *httptest.Server, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req := newTestReq(t, http.MethodPost, ts.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(hdrOrigin, ts.URL)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return do(t, req)
}

// getJSON performs a GET and returns the status code plus the decoded JSON
// body. The response body is closed here; callers must not use the response
// beyond the returned values.
func getJSON(t *testing.T, url string, hdr map[string]string) (int, map[string]any) {
	t.Helper()
	req := newTestReq(t, http.MethodGet, url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res := do(t, req)
	var out map[string]any
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &out)
	_ = res.Body.Close()
	return res.StatusCode, out
}

// cookieHeader names the session cookie header key.
const cookieHeader = "Cookie"

// Frequently asserted endpoint paths and headers in this file.
const (
	epDelete  = "/api/delete"
	epMove    = "/api/move"
	hdrOrigin = "Origin"
)

// login obtains a session cookie for the given username/password.
func loginCookie(t *testing.T, ts *httptest.Server, username, password string) string {
	t.Helper()
	res := postJSON(t, ts, "/api/auth/login", `{"username":`+quote(username)+`,"password":`+quote(password)+`}`, nil)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("login status = %d (body: %s)", res.StatusCode, body)
	}
	for _, c := range res.Cookies() {
		if c.Name == auth.SessionCookieName() {
			return c.Name + "=" + c.Value
		}
	}
	t.Fatal("no session cookie set")
	return ""
}

func quote(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func TestAuthOnboardingFlow(t *testing.T) {
	t.Parallel()
	ts, _ := newAuthTestServer(t, "", "false")

	// Anonymous is not admin.
	code, me := getJSON(t, ts.URL+"/api/me", nil)
	if code != http.StatusOK || me["admin"] != false || me["initialized"] != false {
		t.Fatalf("me = %+v status %d", me, code)
	}

	// Setup works once.
	res := postJSON(t, ts, "/api/auth/setup", `{"password":"correct horse"}`, nil)
	if res.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("setup = %d (body %s)", res.StatusCode, body)
	}
	_ = res.Body.Close()

	// Second setup is rejected.
	res = postJSON(t, ts, "/api/auth/setup", `{"password":"another pass"}`, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", res.StatusCode)
	}

	// Admin mutations require the session cookie.
	res = postJSON(t, ts, epDelete, `{"paths":["x"]}`, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon delete = %d, want 401", res.StatusCode)
	}
	cookie := loginCookie(t, ts, "admin", "correct horse")
	res = postJSON(t, ts, epDelete, `{"paths":["x"]}`, map[string]string{cookieHeader: cookie})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound { // path does not exist
		t.Fatalf("authed delete = %d, want 404", res.StatusCode)
	}

	// me now reports admin for the cookie.
	code, me = getJSON(t, ts.URL+"/api/me", map[string]string{cookieHeader: cookie})
	if code != http.StatusOK || me["admin"] != true {
		t.Fatalf("me with cookie = %+v", me)
	}
}

func TestAdminWriteEndpoints(t *testing.T) {
	t.Parallel()
	ts, cfg := newAuthTestServer(t, "seeded password", "false")
	cookie := loginCookie(t, ts, "admin", "seeded password")
	hdr := map[string]string{cookieHeader: cookie}

	// Create a file as admin, then delete it.
	res := postJSON(t, ts, "/api/file", `{"path":"victim.txt"}`, map[string]string{"X-Capability": "", cookieHeader: cookie})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create file = %d", res.StatusCode)
	}

	res = postJSON(t, ts, epDelete, `{"paths":["victim.txt"]}`, hdr)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("delete = %d (body %s)", res.StatusCode, body)
	}

	// Move.
	res = postJSON(t, ts, "/api/file", `{"path":"mover.txt"}`, map[string]string{"X-Capability": "", cookieHeader: cookie})
	_ = res.Body.Close()
	res = postJSON(t, ts, epMove, `{"from":"mover.txt","to":"sub/renamed.txt"}`, hdr)

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("move = %d (body %s)", res.StatusCode, body)
	}
	_ = res.Body.Close()

	// Save (PUT /api/raw?path=).
	req := newTestReq(t, http.MethodPut, ts.URL+"/api/raw?path=edited.txt", strings.NewReader("new content"))
	req.Header.Set(hdrOrigin, ts.URL)
	req.Header.Set("Cookie", cookie)
	res = do(t, req)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("save = %d (body %s)", res.StatusCode, body)
	}

	// Saved content is visible to anonymous readers.
	res2 := do(t, newTestReq(t, http.MethodGet, ts.URL+"/api/raw?path=edited.txt", nil))
	raw, _ := io.ReadAll(res2.Body)
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusOK || string(raw) != "new content" {
		t.Fatalf("saved content = %q (status %d)", raw, res2.StatusCode)
	}
	_ = cfg
}

func TestAdminWriteRequiresAuth(t *testing.T) {
	t.Parallel()
	ts, _ := newAuthTestServer(t, "seeded password", "false")

	cases := []struct{ method, path, body string }{
		{http.MethodPost, epDelete, `{"paths":["a"]}`},
		{http.MethodPost, epMove, `{"from":"a","to":"b"}`},
		{http.MethodPut, "/api/raw?path=a.txt", "data"},
	}
	for _, c := range cases {
		req := newTestReq(t, c.method, ts.URL+c.path, strings.NewReader(c.body))
		res := do(t, req)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s anon = %d, want 401", c.method, c.path, res.StatusCode)
		}
	}
}

func TestInsecureModeGrantsEverything(t *testing.T) {
	t.Parallel()
	ts, _ := newAuthTestServer(t, "", "true")

	code, me := getJSON(t, ts.URL+"/api/me", nil)
	if code != http.StatusOK || me["admin"] != true || me["insecure"] != true {
		t.Fatalf("me = %+v", me)
	}

	// Delete works without any cookie.
	res := postJSON(t, ts, epDelete, `{"paths":["nothing.txt"]}`, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("insecure delete = %d, want 404 (missing path)", res.StatusCode)
	}
}

func TestCSRFRejected(t *testing.T) {
	t.Parallel()
	ts, _ := newAuthTestServer(t, "seeded password", "false")
	cookie := loginCookie(t, ts, "admin", "seeded password")

	// Valid session but cross-origin Origin header → 403.
	req := newTestReq(t, http.MethodPost, ts.URL+epDelete, strings.NewReader(`{"paths":["a"]}`))
	req.Header.Set(hdrOrigin, "https://evil.example")
	req.Header.Set("Cookie", cookie)
	res := do(t, req)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin = %d, want 403", res.StatusCode)
	}
}

// The Vite dev proxy (and similar reverse proxies) rewrites the Host header,
// so Origin and Host disagree even for genuine same-origin requests.
// Browsers mark those requests with Sec-Fetch-Site: same-origin, which is
// authoritative and must be accepted.
func TestCSRFDevProxyScenario(t *testing.T) {
	t.Parallel()
	ts, _ := newAuthTestServer(t, "seeded password", "false")
	cookie := loginCookie(t, ts, "admin", "seeded password")

	req := newTestReq(t, http.MethodPost, ts.URL+epDelete, strings.NewReader(`{"paths":["a"]}`))
	req.Header.Set(hdrOrigin, "http://localhost:5173") // page origin
	req.Host = "127.0.0.1:8080"                        // proxied Host
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Cookie", cookie)
	res := do(t, req)
	_ = res.Body.Close()
	if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
		t.Fatalf("same-origin via proxy = %d, want accepted", res.StatusCode)
	}

	// A real cross-site browser request (Sec-Fetch-Site: cross-site) stays
	// rejected even when it tries to claim a forwarded host.
	req2 := newTestReq(t, http.MethodPost, ts.URL+epDelete, strings.NewReader(`{"paths":["a"]}`))
	req2.Header.Set(hdrOrigin, "https://evil.example")
	req2.Header.Set("Sec-Fetch-Site", "cross-site")
	req2.Header.Set("Sec-Fetch-Mode", "cors")
	req2.Header.Set("Cookie", cookie)
	res2 := do(t, req2)
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site = %d, want 403", res2.StatusCode)
	}
}

func TestTusOverrideGatedBehindAdmin(t *testing.T) {
	t.Parallel()
	ts, _ := newAuthTestServer(t, "seeded password", "false")
	cookie := loginCookie(t, ts, "admin", "seeded password")

	// Seed an existing file through the admin save endpoint.
	req := newTestReq(t, http.MethodPut, ts.URL+"/api/raw?path=exists.txt", strings.NewReader("original"))
	req.Header.Set(hdrOrigin, ts.URL)
	req.Header.Set("Cookie", cookie)
	res := do(t, req)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("seed save = %d", res.StatusCode)
	}

	// Guest tus create with override=true → 401 (uploads need an account).
	upload := func(hdr map[string]string) (int, string) {
		req := newTestReq(t, http.MethodPost, ts.URL+"/api/tus?path=exists.txt&override=true", nil)
		req.Header.Set("Upload-Length", "3")
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res := do(t, req)
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode, res.Header.Get("Location")
	}
	if got, _ := upload(nil); got != http.StatusUnauthorized {
		t.Fatalf("guest override = %d, want 401", got)
	}
	code, loc := upload(map[string]string{cookieHeader: cookie, hdrOrigin: ts.URL})
	if code != http.StatusCreated {
		t.Fatalf("admin override = %d, want 201", code)
	}
	// Terminate the session so no file handle leaks past cleanup (Windows).
	delReq := newTestReq(t, http.MethodDelete, ts.URL+loc, nil)
	delReq.Header.Set("Cookie", cookie)
	delReq.Header.Set(hdrOrigin, ts.URL)
	delRes := do(t, delReq)
	_ = delRes.Body.Close()
	if delRes.StatusCode != http.StatusNoContent {
		t.Fatalf("terminate = %d, want 204", delRes.StatusCode)
	}
}

func TestSetupOnlyBeforeInit(t *testing.T) {
	t.Parallel()
	ts, _ := newAuthTestServer(t, "", "false")
	code, _ := getJSON(t, ts.URL+"/api/me", nil)
	if code != http.StatusOK {
		t.Fatalf("me = %d", code)
	}
	// Setup with weak password rejected.
	res := postJSON(t, ts, "/api/auth/setup", `{"password":"short"}`, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("weak setup = %d, want 400", res.StatusCode)
	}
}
