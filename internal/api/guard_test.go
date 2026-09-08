package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skidoodle/filebrowser/internal/config"
	"github.com/skidoodle/filebrowser/internal/guard"
	"github.com/skidoodle/filebrowser/internal/storage/local"
)

func mustJSON(t *testing.T, r io.ReadCloser) map[string]any {
	t.Helper()
	defer r.Close()
	var m map[string]any
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	return m
}

func newGuardedServer(t *testing.T, mutateCfg func(*guard.Config)) *httptest.Server {
	t.Helper()
	cfg := &config.Config{
		Root:      t.TempDir(),
		Address:   "127.0.0.1:0",
		MaxUpload: 1 << 20,
		CacheDir:  t.TempDir(),
		Insecure:  true, // isolate the capability gate from account auth
	}
	log := slog.New(slog.DiscardHandler)
	gCfg := guard.Config{
		RequestRate:  1000, // effectively unlimited unless a test says otherwise
		DownloadRate: 1 << 30,
		Difficulty:   0,
	}
	if mutateCfg != nil {
		mutateCfg(&gCfg)
	}
	grd, err := guard.New(gCfg, log)
	if err != nil {
		t.Fatal(err)
	}
	store, err := local.New(cfg.Root)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{Config: cfg, Store: store, Log: log, Guard: grd, Version: "t"})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestMutationsRequireCapability(t *testing.T) {
	t.Parallel()
	ts := newGuardedServer(t, nil)

	// Without capability → 403.
	res := do(t, newTestReq(t, http.MethodPost, ts.URL+"/api/dir",
		bytes.NewReader([]byte(`{"path":"x"}`))))
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", res.StatusCode)
	}

	// Mint a token (browser-like UA → no PoW at difficulty 0).
	req := newTestReq(t, http.MethodGet, ts.URL+"/api/capability", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (test)")
	res2 := do(t, req)
	body := mustJSON(t, res2.Body)
	_ = res2.Body.Close()
	capability, _ := body["capability"].(string)
	if capability == "" {
		t.Fatalf("no capability in response: %#v", body)
	}

	// With capability → 201.
	req3 := newTestReq(t, http.MethodPost, ts.URL+"/api/dir",
		bytes.NewReader([]byte(`{"path":"docs"}`)))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("X-Capability", capability)
	res3 := do(t, req3)
	_ = res3.Body.Close()
	if res3.StatusCode != http.StatusCreated {
		t.Fatalf("with capability status = %d, want 201", res3.StatusCode)
	}

	// Reads stay open without tokens.
	res4 := testGet(t, ts, ts.URL+"/api/list?path=.")
	_ = res4.Body.Close()
	if res4.StatusCode != http.StatusOK {
		t.Fatalf("read status = %d, want 200", res4.StatusCode)
	}
}

func TestCapabilityPowForBots(t *testing.T) {
	t.Parallel()
	ts := newGuardedServer(t, func(c *guard.Config) { c.Difficulty = 2 })

	// curl-like UA gets a challenge, not a token.
	req := newTestReq(t, http.MethodGet, ts.URL+"/api/capability", nil)
	req.Header.Set("User-Agent", "curl/8.4.0")
	res := do(t, req)
	body := mustJSON(t, res.Body)
	_ = res.Body.Close()
	challenge, _ := body["challenge"].(string)
	difficulty, _ := body["difficulty"].(float64)
	if challenge == "" || difficulty == 0 {
		t.Fatalf("expected PoW challenge, got %#v", body)
	}

	// Wrong nonce → 403.
	reqBad := newTestReq(t, http.MethodPost, ts.URL+"/api/capability",
		bytes.NewReader([]byte(`{"challenge":"`+challenge+`","nonce":"nope"}`)))
	reqBad.Header.Set("Content-Type", "application/json")
	resBad := do(t, reqBad)
	_ = resBad.Body.Close()
	if resBad.StatusCode != http.StatusForbidden {
		t.Fatalf("bad proof status = %d, want 403", resBad.StatusCode)
	}

	// Solved PoW → token.
	nonce := guard.Solve(challenge, int(difficulty))
	reqOK := newTestReq(t, http.MethodPost, ts.URL+"/api/capability",
		bytes.NewReader([]byte(`{"challenge":"`+challenge+`","nonce":"`+nonce+`"}`)))
	reqOK.Header.Set("Content-Type", "application/json")
	resOK := do(t, reqOK)
	bodyOK := mustJSON(t, resOK.Body)
	_ = resOK.Body.Close()
	if tok, _ := bodyOK["capability"].(string); tok == "" {
		t.Fatalf("no token after proof: %#v", bodyOK)
	}
}

func TestHoneypotBans(t *testing.T) {
	t.Parallel()
	ts := newGuardedServer(t, nil)

	res := testGet(t, ts, ts.URL+"/.env")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("honeypot status = %d, want 404", res.StatusCode)
	}

	// Same client IP (httptest uses 127.0.0.1 for every request… the ban
	// applies to 127.0.0.1) → subsequent requests are 429.
	res2 := testGet(t, ts, ts.URL+"/api/health")
	_ = res2.Body.Close()
	if res2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("post-honeypot status = %d, want 429", res2.StatusCode)
	}
}

func TestRateLimit429WithoutBan(t *testing.T) {
	t.Parallel()
	ts := newGuardedServer(t, func(c *guard.Config) { c.RequestRate = 5 })

	// Browser-like requests: no UA-based strikes, only throttling.
	get := func() int {
		req := newTestReq(t, http.MethodGet, ts.URL+"/api/health", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (test)")
		res := do(t, req)
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode
	}

	sawLimited := false
	for range 80 {
		if get() == http.StatusTooManyRequests {
			sawLimited = true
		}
	}
	if !sawLimited {
		t.Fatal("rate limit never tripped")
	}

	// Plain reads must be throttled without escalating into a hard ban:
	// after a short pause the client is served again.
	time.Sleep(300 * time.Millisecond)
	if code := get(); code != http.StatusOK {
		t.Fatalf("expected recovery after throttle, got %d", code)
	}

	// A stripped/bot UA is hostile and does escalate to a ban.
	bot := func() int {
		req := newTestReq(t, http.MethodGet, ts.URL+"/api/health", nil)
		req.Header.Set("User-Agent", "python-requests/2.31")
		res := do(t, req)
		defer func() { _ = res.Body.Close() }()
		return res.StatusCode
	}
	banned := false
	for range 10 {
		if bot() == http.StatusTooManyRequests {
			banned = true
			break
		}
		time.Sleep(110 * time.Millisecond)
	}
	if !banned {
		t.Fatal("bot UA should escalate to a ban")
	}
}
