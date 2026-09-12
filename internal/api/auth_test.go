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
	"github.com/skidoodle/filebrowser/internal/guard"
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
	Seed         string
	Insecure     bool
	MaxUpload    int64
	AccessPolicy string
	Guard        bool
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
		Root:         t.TempDir(),
		Address:      "127.0.0.1:0",
		MaxUpload:    maxUpload,
		CacheDir:     t.TempDir(),
		Insecure:     tc.Insecure,
		AccessPolicy: tc.AccessPolicy,
		Guard:        tc.Guard,
	}
	log := slog.New(slog.DiscardHandler)
	fsStore, err := local.New(cfg.Root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Database == "" {
		cfg.Database = filepath.Join(t.TempDir(), "filebrowser.db")
	}
	st, err := store.Open(cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	var mgr *auth.Manager
	if !cfg.Insecure {
		mgr, err = auth.New(filepath.Dir(cfg.Database), cfg.Secret, tc.Seed, st, log)
		if err != nil {
			t.Fatal(err)
		}
	}
	eng, err := authz.New(st)
	if err != nil {
		t.Fatal(err)
	}
	var grd *guard.Guard
	if tc.Guard {
		grd, err = guard.New(guard.Config{
			CacheDir:    t.TempDir(),
			RequestRate: 100,
		}, log)
		if err != nil {
			t.Fatal(err)
		}
	}
	srv, err := New(Options{Config: cfg, Store: fsStore, AppStore: st, Log: log, Version: "test", Auth: mgr, Authz: eng, Guard: grd})
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
	ts, store, _ := newTestServerCfg(t, testServerConfig{Insecure: true})

	if _, err := store.CreateFile(t.Context(), "hello.txt"); err != nil {
		t.Fatal(err)
	}

	code, me := getJSON(t, ts.URL+"/api/me", nil)
	if code != http.StatusOK || me["admin"] != true || me["insecure"] != true {
		t.Fatalf("me = %+v", me)
	}

	// Listing works and returns items in insecure mode.
	code, list := getJSON(t, ts.URL+"/api/list?path=.", nil)
	if code != http.StatusOK {
		t.Fatalf("insecure list = %d, want 200", code)
	}
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("insecure list items = %v, want 1 item", list["items"])
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

	// Guest tus create with override=true → 403 (overwriting requires an account).
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
	if got, _ := upload(nil); got != http.StatusForbidden {
		t.Fatalf("guest override = %d, want 403", got)
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

func TestAccessPolicy(t *testing.T) {
	t.Parallel()
	ts, _, _ := newTestServerCfg(t, testServerConfig{Seed: seedAdminPass})
	adminCookie := loginAs(t, ts, "admin", seedAdminPass)

	// Phases run in order: each changes the policy for the next one,
	// so they must stay sequential.
	testAccessPolicyPublic(t, ts)
	testAccessPolicyReadonly(t, ts, adminCookie)
	testAccessPolicyPrivate(t, ts, adminCookie)
}

// setAccessPolicy updates the access policy; cookie may be empty for
// anonymous requests. Returns the response status code.
func setAccessPolicy(t *testing.T, ts *httptest.Server, policy, cookie string) int {
	t.Helper()
	req := newTestReq(t, http.MethodPut, ts.URL+"/api/settings/policy", strings.NewReader(`{"access_policy":"`+policy+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(hdrOrigin, ts.URL)
	if cookie != "" {
		req.Header.Set(cookieHeader, cookie)
	}
	r := do(t, req)
	_ = r.Body.Close()
	return r.StatusCode
}

// anonCreateFileWithToken creates a file anonymously and returns the
// edit token from the response.
func anonCreateFileWithToken(t *testing.T, ts *httptest.Server, path string) (string, int) {
	t.Helper()
	req := newTestReq(t, http.MethodPost, ts.URL+"/api/file", strings.NewReader(`{"path":"`+path+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(hdrOrigin, ts.URL)
	res := do(t, req)
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusCreated {
		return "", res.StatusCode
	}
	var fileResp struct {
		EditToken string `json:"edit_token"`
	}
	_ = json.NewDecoder(res.Body).Decode(&fileResp)
	return fileResp.EditToken, res.StatusCode
}

// anonSaveWithToken saves content to path using an edit token.
func anonSaveWithToken(t *testing.T, ts *httptest.Server, path, token string) int {
	t.Helper()
	req := newTestReq(t, http.MethodPut, ts.URL+"/api/raw?path="+path, strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set(hdrOrigin, ts.URL)
	req.Header.Set("X-Edit-Token", token)
	res := do(t, req)
	_ = res.Body.Close()
	return res.StatusCode
}

// testAccessPolicyPublic covers anonymous behavior under the default
// public policy: free create/save with edit tokens, delete still denied.
func testAccessPolicyPublic(t *testing.T, ts *httptest.Server) {
	t.Helper()

	// /api/me reports default access policy "public"
	code, body := getJSON(t, ts.URL+"/api/me", nil)
	if code != http.StatusOK {
		t.Fatalf("me = %d", code)
	}
	if body["access_policy"] != "public" {
		t.Fatalf("me.AccessPolicy = %v, want public", body["access_policy"])
	}

	// Anonymous can create a directory
	res := postJSON(t, ts, "/api/dir", `{"path":"pubdir"}`, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("anon create dir = %d, want 201", res.StatusCode)
	}

	// Anonymous create file receives an edit token
	token, status := anonCreateFileWithToken(t, ts, "pubfile.txt")
	if status != http.StatusCreated || token == "" {
		t.Fatalf("anon create file = %d token=%q, want 201 + token", status, token)
	}

	// Anonymous save using edit_token succeeds
	if got := anonSaveWithToken(t, ts, "pubfile.txt", token); got != http.StatusOK {
		t.Fatalf("anon save with edit_token = %d, want 200", got)
	}

	// Anonymous save on different path with same token fails (401)
	if got := anonSaveWithToken(t, ts, "other.txt", token); got != http.StatusUnauthorized {
		t.Fatalf("anon save wrong path = %d, want 401", got)
	}

	// Anonymous delete is rejected
	res = postJSON(t, ts, "/api/delete", `{"paths":["pubfile.txt"]}`, nil)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon delete = %d, want 401", res.StatusCode)
	}
}

// testAccessPolicyReadonly switches to readonly policy: only admins may
// change policy, anonymous users can read but not create.
func testAccessPolicyReadonly(t *testing.T, ts *httptest.Server, adminCookie string) {
	t.Helper()

	if got := setAccessPolicy(t, ts, "readonly", ""); got != http.StatusUnauthorized {
		t.Fatalf("anon set policy = %d, want 401", got)
	}
	if got := setAccessPolicy(t, ts, "readonly", adminCookie); got != http.StatusOK {
		t.Fatalf("admin set policy = %d, want 200", got)
	}

	listCode, _ := getJSON(t, ts.URL+"/api/list?path=.", nil)
	if listCode != http.StatusOK {
		t.Fatalf("anon list in readonly = %d, want 200", listCode)
	}

	req := newTestReq(t, http.MethodPost, ts.URL+"/api/dir", strings.NewReader(`{"path":"readonly_dir"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(hdrOrigin, ts.URL)
	res := do(t, req)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon create dir in readonly = %d, want 401", res.StatusCode)
	}
}

// testAccessPolicyPrivate switches to private policy: anonymous callers
// are locked out entirely, admins keep full access.
func testAccessPolicyPrivate(t *testing.T, ts *httptest.Server, adminCookie string) {
	t.Helper()

	if got := setAccessPolicy(t, ts, "private", adminCookie); got != http.StatusOK {
		t.Fatalf("admin set policy private = %d, want 200", got)
	}

	for _, c := range []struct{ name, url string }{
		{"list", "/api/list?path=."},
		{"meta", "/api/meta?path=pubfile.txt"},
		{"usage", "/api/usage"},
	} {
		code, _ := getJSON(t, ts.URL+c.url, nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("anon %s in private = %d, want 401", c.name, code)
		}
	}

	// Admin with cookie can still list and read in private mode
	adminListCode, _ := getJSON(t, ts.URL+"/api/list?path=.", map[string]string{cookieHeader: adminCookie})
	if adminListCode != http.StatusOK {
		t.Fatalf("admin list in private = %d, want 200", adminListCode)
	}
}

func TestAnonymousCreateAndSaveWithGuard(t *testing.T) {
	t.Parallel()
	ts, _, _ := newTestServerCfg(t, testServerConfig{
		Seed:         seedAdminPass,
		AccessPolicy: "public",
		Guard:        true,
	})

	// 1. Get capability token
	capReq := newTestReq(t, http.MethodGet, ts.URL+"/api/capability", nil)
	capRes := do(t, capReq)
	defer func() { _ = capRes.Body.Close() }()
	if capRes.StatusCode != http.StatusOK {
		t.Fatalf("get capability = %d, want 200", capRes.StatusCode)
	}
	var capData struct {
		Capability string `json:"capability"`
	}
	_ = json.NewDecoder(capRes.Body).Decode(&capData)
	if capData.Capability == "" {
		t.Fatal("expected capability token")
	}

	// 2. Anonymous create file without capability fails (403 capability required)
	createNoCap := newTestReq(t, http.MethodPost, ts.URL+"/api/file", strings.NewReader(`{"path":"guardnote.txt"}`))
	createNoCap.Header.Set("Content-Type", "application/json")
	createNoCap.Header.Set(hdrOrigin, ts.URL)
	resNoCap := do(t, createNoCap)
	_ = resNoCap.Body.Close()
	if resNoCap.StatusCode != http.StatusForbidden {
		t.Fatalf("create without capability = %d, want 403", resNoCap.StatusCode)
	}

	// 3. Anonymous create file with capability succeeds and returns edit_token
	createWithCap := newTestReq(t, http.MethodPost, ts.URL+"/api/file", strings.NewReader(`{"path":"guardnote.txt"}`))
	createWithCap.Header.Set("Content-Type", "application/json")
	createWithCap.Header.Set(hdrOrigin, ts.URL)
	createWithCap.Header.Set("X-Capability", capData.Capability)
	resWithCap := do(t, createWithCap)
	defer func() { _ = resWithCap.Body.Close() }()
	if resWithCap.StatusCode != http.StatusCreated {
		t.Fatalf("create with capability = %d, want 201", resWithCap.StatusCode)
	}
	var fileResp struct {
		EditToken string `json:"edit_token"`
	}
	_ = json.NewDecoder(resWithCap.Body).Decode(&fileResp)
	if fileResp.EditToken == "" {
		t.Fatal("expected edit_token in response")
	}

	// 4. Anonymous save with edit_token BUT without X-Capability fails (403 capability required)
	saveNoCap := newTestReq(t, http.MethodPut, ts.URL+"/api/raw?path=guardnote.txt", strings.NewReader("content"))
	saveNoCap.Header.Set("Content-Type", "text/plain")
	saveNoCap.Header.Set(hdrOrigin, ts.URL)
	saveNoCap.Header.Set("X-Edit-Token", fileResp.EditToken)
	resSaveNoCap := do(t, saveNoCap)
	_ = resSaveNoCap.Body.Close()
	if resSaveNoCap.StatusCode != http.StatusForbidden {
		t.Fatalf("save without capability = %d, want 403", resSaveNoCap.StatusCode)
	}

	// 5. Anonymous save with BOTH edit_token AND X-Capability succeeds (200 OK)
	saveWithBoth := newTestReq(t, http.MethodPut, ts.URL+"/api/raw?path=guardnote.txt", strings.NewReader("saved content"))
	saveWithBoth.Header.Set("Content-Type", "text/plain")
	saveWithBoth.Header.Set(hdrOrigin, ts.URL)
	saveWithBoth.Header.Set("X-Edit-Token", fileResp.EditToken)
	saveWithBoth.Header.Set("X-Capability", capData.Capability)
	resSaveWithBoth := do(t, saveWithBoth)
	_ = resSaveWithBoth.Body.Close()
	if resSaveWithBoth.StatusCode != http.StatusOK {
		t.Fatalf("save with both tokens = %d, want 200", resSaveWithBoth.StatusCode)
	}
}
