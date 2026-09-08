package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Test fixtures shared across the permission tests.
const (
	seedAdminPass = "admin password"
	aliceUser     = "alice"
	alicePass     = "alice password"
	aliceScope    = "alice"
	bobUser       = "bob"
	bobPass       = "bob password"
	bobScope      = "bob"
)

// Frequently asserted endpoint paths and headers in this file.
const epDir = "/api/dir"

func permTestUsers() []testUser {
	return []testUser{
		{Username: aliceUser, Password: alicePass, Scope: aliceScope},
		{Username: bobUser, Password: bobPass, Scope: bobScope},
	}
}

// loginAs signs in as the given user and returns the cookie header value.
func loginAs(t *testing.T, ts *httptest.Server, username, password string) string {
	t.Helper()
	return loginCookie(t, ts, username, password)
}

// getJSONList performs a GET and decodes a JSON array body.
func getJSONList(t *testing.T, url string, hdr map[string]string) (int, []any) {
	t.Helper()
	req := newTestReq(t, http.MethodGet, url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res := do(t, req)
	defer func() { _ = res.Body.Close() }()
	var out []any
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

// loginRoles signs in as admin, alice and bob; returns their cookies.
func loginRoles(t *testing.T, ts *httptest.Server) (admin, alice, bob string) {
	t.Helper()
	return loginAs(t, ts, "admin", seedAdminPass),
		loginAs(t, ts, aliceUser, alicePass),
		loginAs(t, ts, bobUser, bobPass)
}

// doReq issues a request with optional cookie + Origin headers.
func doReq(t *testing.T, ts *httptest.Server, method, path, body, cookie string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := newTestReq(t, method, ts.URL+path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(hdrOrigin, ts.URL)
	if cookie != "" {
		req.Header.Set(cookieHeader, cookie)
	}
	return do(t, req)
}

// expect runs one matrix case and fails on mismatch.
func expect(t *testing.T, ts *httptest.Server, name, method, path, body, cookie string, want int) {
	t.Helper()
	res := doReq(t, ts, method, path, body, cookie)
	_ = res.Body.Close()
	if res.StatusCode != want {
		t.Errorf("%s: %s %s = %d, want %d", name, method, path, res.StatusCode, want)
	}
}

// putFile seeds a file with content as admin.
func putFile(t *testing.T, ts *httptest.Server, cookie, path, content string) {
	t.Helper()
	res := doReq(t, ts, http.MethodPut, "/api/raw?path="+path, content, cookie)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("seed %s = %d (body %s)", path, res.StatusCode, body)
	}
}

// mkDir seeds a directory as admin.
func mkDir(t *testing.T, ts *httptest.Server, cookie, path string) {
	t.Helper()
	res := doReq(t, ts, http.MethodPost, epDir, `{"path":"`+path+`"}`, cookie)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("seed dir %s = %d", path, res.StatusCode)
	}
}

// setPrivate marks a directory private as the given user.
func setPrivate(t *testing.T, ts *httptest.Server, cookie, path string, private bool) {
	t.Helper()
	var res *http.Response
	if private {
		res = doReq(t, ts, http.MethodPost, "/api/private", `{"path":"`+path+`"}`, cookie)
	} else {
		res = doReq(t, ts, http.MethodDelete, "/api/private?path="+path, "", cookie)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("set private(%v) %s = %d", private, path, res.StatusCode)
	}
}

func TestRoleEndpointMatrix(t *testing.T) {
	t.Parallel()
	ts, _, _ := newTestServerCfg(t, testServerConfig{Seed: seedAdminPass, Users: permTestUsers()})
	adminCookie, aliceCookie, _ := loginRoles(t, ts)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		cookie string
		want   int
	}{
		// Anonymous: all writes rejected.
		{"anon create dir", http.MethodPost, epDir, `{"path":"x"}`, "", http.StatusUnauthorized},
		{"anon delete", http.MethodPost, epDelete, `{"paths":["a"]}`, "", http.StatusUnauthorized},
		{"anon save", http.MethodPut, "/api/raw?path=a", "d", "", http.StatusUnauthorized},
		{"anon private", http.MethodPost, "/api/private", `{"path":"a"}`, "", http.StatusUnauthorized},
		{"anon users", http.MethodGet, "/api/users", "", "", http.StatusUnauthorized},
		{"anon password", http.MethodPost, "/api/auth/password", `{"current":"x","password":"yyyyyyyy"}`, "", http.StatusUnauthorized},

		// Scoped alice: in-scope allowed, out-of-scope 403.
		{"alice create dir in scope", http.MethodPost, epDir, `{"path":"alice/new"}`, aliceCookie, http.StatusCreated},
		{"alice create at scope root", http.MethodPost, epDir, `{"path":"alice"}`, aliceCookie, http.StatusForbidden},
		{"alice delete scope root", http.MethodPost, epDelete, `{"paths":["alice"]}`, aliceCookie, http.StatusForbidden},
		{"alice rename scope root", http.MethodPost, epMove, `{"from":"alice","to":"alice2"}`, aliceCookie, http.StatusForbidden},
		{"alice save over scope root", http.MethodPut, "/api/raw?path=alice", "hi", aliceCookie, http.StatusForbidden},
		{"alice create dir out of scope", http.MethodPost, epDir, `{"path":"other/x"}`, aliceCookie, http.StatusForbidden},
		{"alice save in scope", http.MethodPut, "/api/raw?path=alice/doc.txt", "hi", aliceCookie, http.StatusOK},
		{"alice save out of scope", http.MethodPut, "/api/raw?path=other.txt", "hi", aliceCookie, http.StatusForbidden},
		{"alice move out of scope", http.MethodPost, epMove, `{"from":"alice/new","to":"outside/new"}`, aliceCookie, http.StatusForbidden},
		{"alice move in scope", http.MethodPost, epMove, `{"from":"alice/new","to":"alice/renamed"}`, aliceCookie, http.StatusOK},
		{"alice delete in scope", http.MethodPost, epDelete, `{"paths":["alice/renamed"]}`, aliceCookie, http.StatusNoContent},
		{"alice users list", http.MethodGet, "/api/users", "", aliceCookie, http.StatusForbidden},
		{"alice private in scope", http.MethodPost, "/api/private", `{"path":"alice/doc.txt"}`, aliceCookie, http.StatusNoContent},
		{"alice private out of scope", http.MethodPost, "/api/private", `{"path":"other/x"}`, aliceCookie, http.StatusForbidden},

		// Admin: everything allowed anywhere.
		{"admin create dir", http.MethodPost, epDir, `{"path":"anywhere/d"}`, adminCookie, http.StatusCreated},
		{"admin users list", http.MethodGet, "/api/users", "", adminCookie, http.StatusOK},
		{"admin delete", http.MethodPost, epDelete, `{"paths":["anywhere/d","alice/doc.txt"]}`, adminCookie, http.StatusNoContent},
	}
	for _, c := range cases {
		expect(t, ts, c.name, c.method, c.path, c.body, c.cookie, c.want)
	}
}

// privacyFixture seeds a server with alice's private folder marked private.
func privacyFixture(t *testing.T) (*httptest.Server, string, string, string) {
	t.Helper()
	ts, _, _ := newTestServerCfg(t, testServerConfig{Seed: seedAdminPass, Users: permTestUsers()})
	adminCookie, aliceCookie, bobCookie := loginRoles(t, ts)
	mkDir(t, ts, adminCookie, "alice/private")
	mkDir(t, ts, adminCookie, "alice/public")
	putFile(t, ts, adminCookie, "alice/private/hidden.txt", "secret")
	putFile(t, ts, adminCookie, "alice/public/visible.txt", "open")
	setPrivate(t, ts, aliceCookie, "alice/private", true)
	return ts, adminCookie, aliceCookie, bobCookie
}

func TestPrivacyListFiltering(t *testing.T) {
	t.Parallel()
	ts, _, aliceCookie, bobCookie := privacyFixture(t)

	assertHiddenFromList(t, ts, "", "guest")
	assertHiddenFromList(t, ts, bobCookie, "bob")
	assertVisibleToList(t, ts, aliceCookie)
}

func TestPrivacyReadEndpoints404(t *testing.T) {
	t.Parallel()
	ts, _, _, bobCookie := privacyFixture(t)

	for _, url := range []string{"/api/meta?path=alice/private/hidden.txt", "/api/raw?path=alice/private/hidden.txt"} {
		expect(t, ts, "bob "+url, http.MethodGet, url, "", bobCookie, http.StatusNotFound)
	}
}

func TestPrivacySearchNoLeak(t *testing.T) {
	t.Parallel()
	ts, _, aliceCookie, bobCookie := privacyFixture(t)

	if out := searchResults(t, ts, bobCookie); strings.Contains(out, "hidden") {
		t.Fatalf("search leaks for bob: %s", out)
	}
	if out := searchResults(t, ts, ""); strings.Contains(out, "hidden") {
		t.Fatalf("search leaks for guest: %s", out)
	}
	if out := searchResults(t, ts, aliceCookie); !strings.Contains(out, "hidden.txt") {
		t.Fatalf("owner search misses own file: %s", out)
	}
}

func TestPrivacyDownloadHidden404(t *testing.T) {
	t.Parallel()
	ts, _, _, _ := privacyFixture(t)

	expect(t, ts, "guest download private", http.MethodGet, "/api/download?path=alice/private", "", "", http.StatusNotFound)
}

func TestPrivacyUnsetReveals(t *testing.T) {
	t.Parallel()
	ts, _, aliceCookie, _ := privacyFixture(t)

	setPrivate(t, ts, aliceCookie, "alice/private", false)
	expect(t, ts, "guest raw after unset", http.MethodGet, "/api/raw?path=alice/private/hidden.txt", "", "", http.StatusOK)
}

// assertHiddenFromList checks the private dir is absent from alice's listing.
func assertHiddenFromList(t *testing.T, ts *httptest.Server, cookie, name string) {
	t.Helper()
	hdr := map[string]string{}
	if cookie != "" {
		hdr[cookieHeader] = cookie
	}
	code, body := getJSON(t, ts.URL+"/api/list?path=alice", hdr)
	if code != http.StatusOK {
		t.Fatalf("list as %s = %d", name, code)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "private") {
		t.Fatalf("%s listing leaks private dir: %s", name, raw)
	}
}

// assertVisibleToList checks the owner still sees the private dir.
func assertVisibleToList(t *testing.T, ts *httptest.Server, cookie string) {
	t.Helper()
	code, body := getJSON(t, ts.URL+"/api/list?path=alice", map[string]string{cookieHeader: cookie})
	if code != http.StatusOK {
		t.Fatal(code)
	}
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), "private") {
		t.Fatalf("owner listing missing private dir: %s", raw)
	}
}

// searchResults returns the raw NDJSON search output.
func searchResults(t *testing.T, ts *httptest.Server, cookie string) string {
	t.Helper()
	req := newTestReq(t, http.MethodGet, ts.URL+"/api/search?q=hidden", nil)
	if cookie != "" {
		req.Header.Set(cookieHeader, cookie)
	}
	res := do(t, req)
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

func TestScopedTusUploads(t *testing.T) {
	t.Parallel()
	ts, _, _ := newTestServerCfg(t, testServerConfig{Seed: seedAdminPass, Users: permTestUsers()})
	_, aliceCookie, bobCookie := loginRoles(t, ts)

	tusCreate := func(path, cookie string) int {
		req := newTestReq(t, http.MethodPost, ts.URL+"/api/tus?path="+path, nil)
		req.Header.Set("Upload-Length", "4")
		if cookie != "" {
			req.Header.Set(cookieHeader, cookie)
			req.Header.Set(hdrOrigin, ts.URL)
		}
		res := do(t, req)
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode == http.StatusCreated {
			// Terminate so no file handle leaks past t.TempDir cleanup.
			terminate(t, ts, res.Header.Get("Location"), cookie)
		}
		return res.StatusCode
	}

	if got := tusCreate("alice/in.txt", aliceCookie); got != http.StatusCreated {
		t.Fatalf("alice in-scope tus = %d, want 201", got)
	}
	if got := tusCreate("outside.txt", aliceCookie); got != http.StatusForbidden {
		t.Fatalf("alice out-of-scope tus = %d, want 403", got)
	}
	if got := tusCreate("alice/in.txt", bobCookie); got != http.StatusForbidden {
		t.Fatalf("bob into alice's scope = %d, want 403", got)
	}
	if got := tusCreate("alice/in.txt", ""); got != http.StatusUnauthorized {
		t.Fatalf("guest tus = %d, want 401", got)
	}
}

// terminate aborts an upload session (Windows file-handle hygiene).
func terminate(t *testing.T, ts *httptest.Server, loc, cookie string) {
	t.Helper()
	delReq := newTestReq(t, http.MethodDelete, ts.URL+loc, nil)
	if cookie != "" {
		delReq.Header.Set(cookieHeader, cookie)
		delReq.Header.Set(hdrOrigin, ts.URL)
	}
	res := do(t, delReq)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("terminate = %d, want 204", res.StatusCode)
	}
}

// usersFixture creates a server with admin signed in and carol created.
func usersFixture(t *testing.T) (*httptest.Server, string, map[string]string) {
	t.Helper()
	ts, _, _ := newTestServerCfg(t, testServerConfig{Seed: seedAdminPass})
	adminCookie := loginAs(t, ts, "admin", seedAdminPass)
	adminHdr := map[string]string{cookieHeader: adminCookie, hdrOrigin: ts.URL}
	return ts, adminCookie, adminHdr
}

func TestUsersCRUDAndGuardrails(t *testing.T) {
	t.Parallel()
	ts, _, adminHdr := usersFixture(t)

	carolID := createCarol(t, ts, adminHdr)
	if carolID == 0 {
		t.Fatal("carol not created")
	}

	// Duplicate username → 409.
	res := postJSON(t, ts, "/api/users", `{"username":"carol","password":"carol password","admin":false,"scope":""}`, adminHdr)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate username = %d, want 409", res.StatusCode)
	}

	// Admin resets carol's password and renames her.
	req := newTestReq(t, http.MethodPatch, ts.URL+"/api/users/"+intToStr(carolID), strings.NewReader(`{"password":"reset pass","username":"carla"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(hdrOrigin, ts.URL)
	req.Header.Set(cookieHeader, loginCookieHeader(adminHdr))
	res = do(t, req)
	_ = res.Body.Close()
	// A pure password reset answers 204; a rename (or other attribute
	// change) answers 200 with the updated user.
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		t.Fatalf("admin reset password = %d", res.StatusCode)
	}
	if c := loginAs(t, ts, "carla", "reset pass"); c == "" {
		t.Fatal("login with renamed user + reset password failed")
	}

	// Delete guardrails: original admin protected, carol deletable.
	adminCookie := loginCookieHeader(adminHdr)
	expectDeleteUser(t, ts, adminCookie, findUser(t, ts, adminHdr, "admin", true), http.StatusConflict)
	expectDeleteUser(t, ts, adminCookie, carolID, http.StatusNoContent)
}

// loginCookieHeader extracts the cookie value from the fixture header map.
func loginCookieHeader(hdr map[string]string) string { return hdr[cookieHeader] }

// createCarol creates a scoped user and returns the id.
func createCarol(t *testing.T, ts *httptest.Server, adminHdr map[string]string) int64 {
	t.Helper()
	res := postJSON(t, ts, "/api/users", `{"username":"carol","password":"carol password","admin":false,"scope":"carol"}`, adminHdr)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create user = %d", res.StatusCode)
	}
	id := findUser(t, ts, adminHdr, "carol", false)
	if id == 0 {
		t.Fatal("carol not in list")
	}
	return id
}

func TestSelfUsernameChange(t *testing.T) {
	t.Parallel()
	ts, _, _ := newTestServerCfg(t, testServerConfig{Seed: seedAdminPass})
	adminCookie := loginAs(t, ts, "admin", seedAdminPass)
	adminHdr := map[string]string{cookieHeader: adminCookie, "Origin": ts.URL}
	createCarol(t, ts, adminHdr)
	carolCookie := loginAs(t, ts, "carol", "carol password")
	carolHdr := map[string]string{cookieHeader: carolCookie, "Origin": ts.URL}

	// Rename self without password.
	res := postJSON(t, ts, "/api/auth/password", `{"username":"carolyn"}`, carolHdr)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("self rename = %d", res.StatusCode)
	}
	if c := loginAs(t, ts, "carolyn", "carol password"); c == "" {
		t.Fatal("login with renamed account failed")
	}

	// me reports the new username.
	code, me := getJSON(t, ts.URL+"/api/me", carolHdr)
	if code != http.StatusOK || me["username"] != "carolyn" {
		t.Fatalf("me after rename = %+v (code %d)", me, code)
	}

	// Taken name → 409.
	res = postJSON(t, ts, "/api/auth/password", `{"username":"admin"}`, carolHdr)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("rename to taken name = %d, want 409", res.StatusCode)
	}
}

func TestPrivateFlagAnnotatedForOwner(t *testing.T) {
	t.Parallel()
	ts, _, aliceCookie, _ := privacyFixture(t)

	// The owner's listing must carry private:true on the private dir so
	// the context menu can offer "Make public".
	code, body := getJSON(t, ts.URL+"/api/list?path=alice", map[string]string{cookieHeader: aliceCookie})
	if code != http.StatusOK {
		t.Fatal(code)
	}
	raw, _ := json.Marshal(body)
	if !strings.Contains(string(raw), `"private":true`) {
		t.Fatalf("owner listing missing private flag: %s", raw)
	}
	// Guests see no private flag (they don't see the folder at all).
	code, body = getJSON(t, ts.URL+"/api/list?path=alice", nil)
	if code != http.StatusOK {
		t.Fatal(code)
	}
	raw, _ = json.Marshal(body)
	if strings.Contains(string(raw), "private") {
		t.Fatalf("guest listing leaks private dir: %s", raw)
	}
}

func TestSelfPasswordChange(t *testing.T) {
	t.Parallel()
	ts, _, _ := newTestServerCfg(t, testServerConfig{Seed: seedAdminPass})
	adminCookie := loginAs(t, ts, "admin", seedAdminPass)
	adminHdr := map[string]string{cookieHeader: adminCookie, hdrOrigin: ts.URL}
	createCarol(t, ts, adminHdr)
	carolCookie := loginAs(t, ts, "carol", "carol password")

	// Success.
	res := postJSON(t, ts, "/api/auth/password", `{"current":"carol password","password":"carol new pass"}`, map[string]string{cookieHeader: carolCookie, hdrOrigin: ts.URL})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("self password change = %d", res.StatusCode)
	}
	if c := loginAs(t, ts, "carol", "carol new pass"); c == "" {
		t.Fatal("login with new password failed")
	}

	// Wrong current password → 403.
	carolCookie2 := loginAs(t, ts, "carol", "carol new pass")
	res = postJSON(t, ts, "/api/auth/password", `{"current":"wrong","password":"carol new pass"}`, map[string]string{cookieHeader: carolCookie2, hdrOrigin: ts.URL})
	_ = res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong current = %d, want 403", res.StatusCode)
	}
}

// findUser returns the id of the named user from the admin list.
func findUser(t *testing.T, ts *httptest.Server, adminHdr map[string]string, name string, original bool) int64 {
	t.Helper()
	code, users := getJSONList(t, ts.URL+"/api/users", adminHdr)
	if code != http.StatusOK {
		t.Fatalf("users list = %d", code)
	}
	for _, u := range users {
		m, ok := u.(map[string]any)
		if !ok || m["username"] != name {
			continue
		}
		if original {
			if isOrig, _ := m["isOriginal"].(bool); isOrig {
				return toID(t, m["id"])
			}
			continue
		}
		return toID(t, m["id"])
	}
	return 0
}

func toID(t *testing.T, v any) int64 {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("unexpected id type %T", v)
	}
	return int64(f)
}

// expectDeleteUser deletes a user and checks the status.
func expectDeleteUser(t *testing.T, ts *httptest.Server, adminCookie string, id int64, want int) {
	t.Helper()
	req := newTestReq(t, http.MethodDelete, ts.URL+"/api/users/"+intToStr(id), nil)
	req.Header.Set(hdrOrigin, ts.URL)
	req.Header.Set(cookieHeader, adminCookie)
	res := do(t, req)
	_ = res.Body.Close()
	if res.StatusCode != want {
		t.Fatalf("delete user %d = %d, want %d", id, res.StatusCode, want)
	}
}

func TestUserDeletionRevealsPrivateFolders(t *testing.T) {
	t.Parallel()
	ts, _, _ := newTestServerCfg(t, testServerConfig{
		Seed:  seedAdminPass,
		Users: permTestUsers(),
	})
	adminCookie, aliceCookie, _ := loginRoles(t, ts)
	adminHdr := map[string]string{cookieHeader: adminCookie, hdrOrigin: ts.URL}

	mkDir(t, ts, adminCookie, "alice/priv")
	setPrivate(t, ts, aliceCookie, "alice/priv", true)

	expect(t, ts, "guest meta private", http.MethodGet, "/api/meta?path=alice/priv", "", "", http.StatusNotFound)

	// Delete alice; her private folder becomes visible again (documented).
	expectDeleteUser(t, ts, adminCookie, findUser(t, ts, adminHdr, aliceUser, false), http.StatusNoContent)
	expect(t, ts, "guest meta after owner deletion", http.MethodGet, "/api/meta?path=alice/priv", "", "", http.StatusOK)
}

// intToStr formats a small int for path values.
func intToStr(n int64) string { return strconv.FormatInt(n, 10) }
