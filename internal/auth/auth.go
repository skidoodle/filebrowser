package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/skidoodle/filebrowser/internal/store"
)

// Manager owns accounts, sessions and login backoff.
type Manager struct {
	dir string
	log *slog.Logger
	key []byte
	st  *store.Store

	mu         sync.Mutex
	loginFails map[string]*backoffState
}

// backoffState tracks failed logins per IP with exponential backoff.
type backoffState struct {
	fails    int
	nextOK   time.Time
	lastFail time.Time
}

// Identity describes the authenticated caller.
type Identity struct {
	UserID   int64
	Username string
	Admin    bool
	Scope    string
}

// Guest is the zero identity: not signed in.
var Guest = Identity{}

const (
	sessionCookie = "filebrowser_session"
	sessionTTL    = 7 * 24 * time.Hour
	minPassword   = 8
	maxUsername   = 64
	bcryptCost    = 12
	maxFails      = 5
)

// Sentinel errors.
var (
	ErrUninitialized      = errors.New("auth: no accounts exist yet")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrLocked             = errors.New("auth: too many failed attempts")
	ErrWeakPassword       = errors.New("auth: password too short")
	ErrAlreadyExists      = errors.New("auth: account already exists")
	ErrBadUsername        = errors.New("auth: invalid username")
)

// New creates a Manager persisting accounts in st and resolving the
// session signing key: explicit secret (hashed) first, then a key persisted
// in dir, then fresh. seedPassword (FILEBROWSER_ADMIN_PASSWORD) is
// authoritative: it creates or resets the original admin account.
func New(dir, secret, seedPassword string, st *store.Store, log *slog.Logger) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("auth: create state dir: %w", err)
	}
	key, err := resolveKey(secret, dir)
	if err != nil {
		return nil, err
	}
	m := &Manager{
		dir:        dir,
		log:        log,
		key:        key,
		st:         st,
		loginFails: make(map[string]*backoffState),
	}
	if seedPassword != "" {
		if len(seedPassword) < minPassword {
			return nil, fmt.Errorf("auth: FILEBROWSER_ADMIN_PASSWORD: %w (min %d chars)", ErrWeakPassword, minPassword)
		}
		h, err := bcrypt.GenerateFromPassword([]byte(seedPassword), bcryptCost)
		if err != nil {
			return nil, fmt.Errorf("auth: hash seed password: %w", err)
		}
		if _, err := st.CreateOriginal(h); err != nil {
			return nil, err
		}
		log.Info("seeded original admin password from FILEBROWSER_ADMIN_PASSWORD")
	}
	return m, nil
}

// Initialized reports whether any account exists.
func (m *Manager) Initialized() bool {
	n, err := m.st.CountUsers()
	return err == nil && n > 0
}

// Store returns the underlying SQLite store.
func (m *Manager) Store() *store.Store {
	return m.st
}

// hashPassword bcrypts a password with the package cost.
func hashPassword(password string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
}

// Setup creates the original admin account. It fails if any account exists.
func (m *Manager) Setup(password string) error {
	if len(password) < minPassword {
		return ErrWeakPassword
	}
	if m.Initialized() {
		return ErrAlreadyExists
	}
	h, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("auth: hash password: %w", err)
	}
	if _, err := m.st.CreateOriginal(h); err != nil {
		if errors.Is(err, store.ErrExists) {
			return ErrAlreadyExists
		}
		return err
	}
	return nil
}

// SessionAfterSetup mints a session immediately following account creation
// (the caller has just proven control of the new password).
func (m *Manager) SessionAfterSetup() (string, error) {
	u, err := m.st.UserByName("admin")
	if err != nil {
		return "", ErrUninitialized
	}
	return m.newSession(u.ID), nil
}

// Login verifies username and password (with per-IP backoff) and returns a
// session token to set as a cookie.
func (m *Manager) Login(ip, username, password string) (string, error) {
	m.mu.Lock()
	if st := m.loginFails[ip]; st != nil && time.Now().Before(st.nextOK) {
		m.mu.Unlock()
		return "", ErrLocked
	}
	m.mu.Unlock()

	if !m.Initialized() {
		return "", ErrUninitialized
	}
	u, err := m.st.UserByName(username)
	if err != nil {
		// Burn bcrypt time even for unknown usernames to keep the timing
		// indistinguishable from a wrong password.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		m.registerFail(ip)
		return "", ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password)) != nil {
		m.registerFail(ip)
		return "", ErrInvalidCredentials
	}
	m.clearFails(ip)
	return m.newSession(u.ID), nil
}

// dummyHash is a fixed bcrypt hash so unknown usernames cost the same as
// known ones during login.
var dummyHash = func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("filebrowser-dummy-password"), bcryptCost)
	if err != nil {
		panic("auth: dummy hash: " + err.Error())
	}
	return h
}()

// registerFail records a failed attempt. Backoff engages only after
// maxFails consecutive failures and grows exponentially, capped at 1h.
func (m *Manager) registerFail(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.loginFails[ip]
	if st == nil {
		st = &backoffState{}
		m.loginFails[ip] = st
	}
	now := time.Now()
	if now.Sub(st.lastFail) > time.Hour {
		st.fails = 0 // stale streak: start over
	}
	st.fails++
	st.lastFail = now
	if st.fails >= maxFails {
		wait := time.Duration(1<<min(st.fails-maxFails+1, 12)) * time.Second
		st.nextOK = now.Add(wait)
	}
}

func (m *Manager) clearFails(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.loginFails, ip)
}

// newSession mints a stateless signed session token bound to a user:
// hex(exp).hex(uid).hex(mac).
func (m *Manager) newSession(uid int64) string {
	exp := strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 16)
	uidHex := strconv.FormatInt(uid, 16)
	mac := hmacSHA256(m.key, []byte(exp+"."+uidHex))
	return exp + "." + uidHex + "." + hex.EncodeToString(mac)
}

// VerifySession validates a session token and resolves the current account.
// Accounts deleted or demoted after login lose access immediately because
// the user row is reloaded on every verification.
func (m *Manager) VerifySession(token string) (Identity, error) {
	expHex, rest, ok := strings.Cut(token, ".")
	if !ok {
		return Identity{}, ErrInvalidCredentials
	}
	uidHex, macHex, ok := strings.Cut(rest, ".")
	if !ok {
		return Identity{}, ErrInvalidCredentials
	}
	mac, err := hex.DecodeString(macHex)
	if err != nil || !hmac.Equal(hmacSHA256(m.key, []byte(expHex+"."+uidHex)), mac) {
		return Identity{}, ErrInvalidCredentials
	}
	exp, err := strconv.ParseInt(expHex, 16, 64)
	if err != nil || time.Now().Unix() > exp {
		return Identity{}, ErrInvalidCredentials
	}
	uid, err := strconv.ParseInt(uidHex, 16, 64)
	if err != nil {
		return Identity{}, ErrInvalidCredentials
	}
	u, err := m.st.User(uid)
	if err != nil {
		return Identity{}, ErrInvalidCredentials
	}
	return Identity{UserID: u.ID, Username: u.Username, Admin: u.Admin, Scope: u.Scope}, nil
}

// CreateUser adds an account (admin-only operation, enforced by the API).
func (m *Manager) CreateUser(username, password string, admin bool, scope string) (store.User, error) {
	if len(password) < minPassword {
		return store.User{}, ErrWeakPassword
	}
	if !store.ValidUsername(username) {
		return store.User{}, ErrBadUsername
	}
	h, err := hashPassword(password)
	if err != nil {
		return store.User{}, fmt.Errorf("auth: hash password: %w", err)
	}
	return m.st.CreateUser(username, h, admin, scope)
}

// UpdateUser changes an account's admin flag and scope.
func (m *Manager) UpdateUser(id int64, admin bool, scope string) (store.User, error) {
	return m.st.UpdateUser(id, admin, scope)
}

// RenameUser changes an account's username.
func (m *Manager) RenameUser(id int64, username string) (store.User, error) {
	if !store.ValidUsername(username) {
		return store.User{}, ErrBadUsername
	}
	return m.st.RenameUser(id, username)
}

// User loads a single account by id.
func (m *Manager) User(id int64) (store.User, error) { return m.st.User(id) }

// DeleteUser removes an account (guardrails live in the store).
func (m *Manager) DeleteUser(id int64) error { return m.st.DeleteUser(id) }

// ListUsers returns every account.
func (m *Manager) ListUsers() ([]store.User, error) { return m.st.ListUsers() }

// AdminSetPassword resets any account's password without the old one.
func (m *Manager) AdminSetPassword(id int64, password string) error {
	if len(password) < minPassword {
		return ErrWeakPassword
	}
	h, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("auth: hash password: %w", err)
	}
	return m.st.SetPassword(id, h)
}

// ChangePassword verifies the current password before setting a new one.
func (m *Manager) ChangePassword(id int64, oldPassword, newPassword string) error {
	if len(newPassword) < minPassword {
		return ErrWeakPassword
	}
	u, err := m.st.User(id)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	h, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("auth: hash password: %w", err)
	}
	return m.st.SetPassword(id, h)
}

// SessionCookieName is the cookie the SPA relies on.
func SessionCookieName() string { return sessionCookie }

// SessionTTL exposes the session lifetime (sliding refresh happens in the API).
func SessionTTL() time.Duration { return sessionTTL }

// MinPasswordLen is the shortest accepted password.
func MinPasswordLen() int { return minPassword }

// MaxUsernameLen is the longest accepted username.
func MaxUsernameLen() int { return maxUsername }

func hmacSHA256(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

// resolveKey mirrors guard.ResolveKey: explicit secret (hashed), persisted
// key file, or fresh random key.
func resolveKey(secret, dir string) ([]byte, error) {
	if secret != "" {
		sum := sha256.Sum256([]byte(secret))
		return sum[:], nil
	}
	path := dir + "/session.key"
	if raw, err := os.ReadFile(path); err == nil {
		if key, err := hex.DecodeString(strings.TrimSpace(string(raw))); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("auth: generate key: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, fmt.Errorf("auth: persist key: %w", err)
	}
	return key, nil
}

// ClientIP extracts the best-effort client IP for login backoff. It is
// deliberately simple: backoff is a nuisance control, not a security
// boundary; the request limiter already caps request rates.
func ClientIP(remoteAddr string) netip.Addr {
	host, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		addr, err := netip.ParseAddr(remoteAddr)
		if err != nil {
			return netip.Addr{}
		}
		return addr.Unmap()
	}
	return host.Addr().Unmap()
}

// HashPassword is exported for tests and account tooling.
func HashPassword(password string) ([]byte, error) { return hashPassword(password) }

// ComparePassword is exported for tests.
func ComparePassword(hash []byte, password string) bool {
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

// ConstantTimeEqual is a helper for comparing CSRF nonces etc.
func ConstantTimeEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
