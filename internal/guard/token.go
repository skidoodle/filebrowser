package guard

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Tokens issues and verifies HMAC-signed, short-lived capability tokens.
// They prove a request came from JavaScript that loaded the app, without
// any user-visible friction.
type Tokens struct {
	key []byte
}

// ResolveKey returns the capability signing key, in order of preference:
// an explicit secret (hashed), a key persisted in cacheDir (survives
// restarts), or a freshly generated one. Persistence matters: a random
// per-process key invalidates every browser's cached capability token
// whenever the server restarts, surfacing as 403s on the next upload.
func ResolveKey(secret, cacheDir string) ([]byte, error) {
	if secret != "" {
		sum := sha256.Sum256([]byte(secret))
		return sum[:], nil
	}

	if cacheDir == "" {
		return randomKey()
	}

	path := filepath.Join(cacheDir, "capability.key")
	if raw, err := os.ReadFile(path); err == nil {
		if key, err := hex.DecodeString(strings.TrimSpace(string(raw))); err == nil && len(key) == 32 {
			return key, nil
		}
	}

	key, err := randomKey()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func randomKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// NewTokens creates a token signer with a fresh random key.
func NewTokens() (*Tokens, error) {
	key, err := ResolveKey("", "")
	if err != nil {
		return nil, err
	}
	return &Tokens{key: key}, nil
}

// NewTokensWithKey creates a signer from an existing key so capability
// tokens remain valid across server restarts.
func NewTokensWithKey(key []byte) *Tokens {
	return &Tokens{key: key}
}

// Issue mints a capability token valid for dur.
func (t *Tokens) Issue(dur time.Duration) string {
	exp := strconv.FormatInt(time.Now().Add(dur).Unix(), 16)
	mac := t.mac(exp)
	return exp + "." + hex.EncodeToString(mac)
}

// Verify checks signature and expiry.
func (t *Tokens) Verify(token string) bool {
	expHex, macHex, ok := strings.Cut(token, ".")
	if !ok || len(macHex) != sha256.Size*2 {
		return false
	}
	mac, err := hex.DecodeString(macHex)
	if err != nil {
		return false
	}
	if !hmac.Equal(t.mac(expHex), mac) {
		return false
	}
	exp, err := strconv.ParseInt(expHex, 16, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return true
}

func (t *Tokens) mac(exp string) []byte {
	m := hmac.New(sha256.New, t.key)
	m.Write([]byte(exp))
	return m.Sum(nil)
}

// Pow is a hashcash-style proof-of-work: find a nonce such that
// sha256(challenge + ":" + nonce) starts with `difficulty` hex zeros.
type Pow struct{}

// CheckDifficulty reports whether a solution has enough leading zeros.
func CheckDifficulty(sumHex string, difficulty int) bool {
	if difficulty <= 0 {
		return true
	}
	return strings.Count(sumHex[:difficulty], "0") == difficulty
}

// Solve is a reference (non-JS) solver used by tests.
func Solve(challenge string, difficulty int) string {
	for i := uint64(0); ; i++ {
		nonce := strconv.FormatUint(i, 10)
		sum := sha256.Sum256([]byte(challenge + ":" + nonce))
		if CheckDifficulty(hex.EncodeToString(sum[:]), difficulty) {
			return nonce
		}
	}
}

// RandomChallenge returns a random challenge string.
func RandomChallenge() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
