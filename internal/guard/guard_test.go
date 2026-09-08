package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestTokensIssueVerify(t *testing.T) {
	t.Parallel()
	tk, err := NewTokens()
	if err != nil {
		t.Fatal(err)
	}
	tok := tk.Issue(time.Minute)
	if !tk.Verify(tok) {
		t.Fatal("fresh token should verify")
	}
	// Tampered signature.
	if tk.Verify(tok[:len(tok)-2] + "ff") {
		t.Fatal("tampered token should fail")
	}
	// Cross-key token.
	tk2, _ := NewTokens()
	if tk2.Verify(tok) {
		t.Fatal("token from another key should fail")
	}
	// Expired.
	expired := tk.Issue(-time.Second)
	if tk.Verify(expired) {
		t.Fatal("expired token should fail")
	}
}

func TestPow(t *testing.T) {
	t.Parallel()
	challenge, err := RandomChallenge()
	if err != nil {
		t.Fatal(err)
	}
	nonce := Solve(challenge, 3)
	sum := sha256.Sum256([]byte(challenge + ":" + nonce))
	if !CheckDifficulty(hex.EncodeToString(sum[:]), 3) {
		t.Fatal("solver output should satisfy difficulty")
	}
	if CheckDifficulty("abc123", 3) {
		t.Fatal("insufficient zeros should fail")
	}
	if !CheckDifficulty("abc123", 0) {
		t.Fatal("difficulty 0 should always pass")
	}
}

func TestRateLimiter(t *testing.T) {
	t.Parallel()
	l := newLimiter(10, 5) // 10/s, burst 5
	for range 5 {
		if !l.allow("a", 1) {
			t.Fatal("burst request should pass")
		}
	}
	if l.allow("a", 1) {
		t.Fatal("request beyond burst should fail")
	}
	// Different key unaffected.
	if !l.allow("b", 1) {
		t.Fatal("other key should pass")
	}
	// Refill over time.
	time.Sleep(120 * time.Millisecond) // ~1.2 tokens
	if !l.allow("a", 1) {
		t.Fatal("refilled token should allow one more")
	}
}

func TestBansEscalate(t *testing.T) {
	t.Parallel()
	b := newBans()
	if banned, _ := b.Banned("ip1"); banned {
		t.Fatal("clean key should not be banned")
	}
	for range strikeThreshold {
		b.Strike("ip1")
	}
	banned, dur := b.Banned("ip1")
	if !banned || dur <= 0 {
		t.Fatal("threshold strikes should trigger a ban")
	}
	// Escalation: next cycle doubles.
	for range strikeThreshold {
		b.Strike("ip1")
	}
	_, dur2 := b.Banned("ip1")
	if dur2 <= dur {
		t.Fatalf("ban should escalate: %v then %v", dur, dur2)
	}
}

func TestClientIPTrustedProxy(t *testing.T) {
	t.Parallel()
	trusted := []string{"10.0.0.0/8"}

	mk := func(remote, xff string) *http.Request {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}

	// Direct client, no proxy: ignore any spoofed header.
	if got := ClientIP(mk("203.0.113.9:1234", "1.2.3.4"), prefixes(trusted)); got.String() != "203.0.113.9" {
		t.Fatalf("untrusted XFF honored: %v", got)
	}
	// Trusted proxy: believe the last untrusted hop.
	if got := ClientIP(mk("10.0.0.2:1234", "198.51.100.7, 10.0.0.3"), prefixes(trusted)); got.String() != "198.51.100.7" {
		t.Fatalf("got %v, want 198.51.100.7", got)
	}
	// Spoofed XFF from an untrusted chain end stays at the proxy.
	if got := ClientIP(mk("10.0.0.2:1234", "1.2.3.4"), prefixes(trusted)); got.String() != "1.2.3.4" {
		t.Fatalf("trusted proxy should take leftmost: %v", got)
	}
}

func TestAllowPartialBandwidth(t *testing.T) {
	t.Parallel()
	l := newLimiter(1<<20, 1<<20) // 1 MiB/s, burst 1 MiB

	// A 10 MiB request against a 1 MiB burst must be allowed (consumed
	// partially), unlike a strict bucket.
	if !l.allowPartial("dl", 10<<20, 64<<10) {
		t.Fatal("oversized payload should be allowed with partial charge")
	}
	if l.allowPartial("dl", 10<<20, 64<<10) {
		t.Fatal("drained bucket should reject")
	}
	// Strict bucket still blocks over-burst requests.
	if l.allow("dl", 5<<20) {
		t.Fatal("strict allow should reject beyond burst")
	}
}

func TestResolveKeyPersistence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	k1, err := ResolveKey("", dir)
	if err != nil {
		t.Fatal(err)
	}
	// Second boot must read the same persisted key, or every open browser
	// session would 403 on its next mutation.
	k2, err := ResolveKey("", dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(k1) != string(k2) {
		t.Fatal("persisted capability key changed across restarts")
	}
	if len(k1) != 32 {
		t.Fatalf("key length = %d, want 32", len(k1))
	}

	// Explicit secret is deterministic and independent of the cache dir.
	s1, err := ResolveKey("hunter2", dir)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := ResolveKey("hunter2", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if string(s1) != string(s2) {
		t.Fatal("same secret must derive the same key")
	}
	if string(s1) == string(k1) {
		t.Fatal("secret-derived key must differ from the random persisted one")
	}

	// Tokens minted by one instance verify in a "restarted" instance.
	tk1 := NewTokensWithKey(k1)
	tk2 := NewTokensWithKey(k2)
	if !tk2.Verify(tk1.Issue(time.Hour)) {
		t.Fatal("token must survive a restart with a persisted key")
	}
}

func TestHoneypotAndUA(t *testing.T) {
	t.Parallel()
	for _, p := range []string{"/.env", "/wp-login.php", "/.git/config", "/cgi-bin/foo"} {
		if !honeypot(p) {
			t.Errorf("%q should be a honeypot", p)
		}
	}
	if honeypot("/docs/file.txt") {
		t.Error("normal path must not be a honeypot")
	}
	if !suspiciousUserAgent("") || !suspiciousUserAgent("curl/8.0") {
		t.Error("empty/curl UA should be suspicious")
	}
	if suspiciousUserAgent("Mozilla/5.0 (Windows NT 10.0) Chrome/130.0") {
		t.Error("browser UA should not be suspicious")
	}
}

func prefixes(cidrs []string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(c)
		if err == nil {
			out = append(out, p.Masked())
		}
	}
	return out
}
