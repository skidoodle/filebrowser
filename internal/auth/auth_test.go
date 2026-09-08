package auth

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/skidoodle/filebrowser/internal/store"
)

// newTestStore opens a SQLite store inside dir.
func newTestStore(t *testing.T, dir string) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	m, err := New(dir, "", "", newTestStore(t, dir), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSetupAndLogin(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)

	if m.Initialized() {
		t.Fatal("fresh manager must be uninitialized")
	}
	if _, err := m.Login("1.2.3.4", "admin", "whatever"); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("login before setup = %v, want ErrUninitialized", err)
	}
	if err := m.Setup("short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("setup short password = %v, want ErrWeakPassword", err)
	}
	if err := m.Setup("correct horse"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := m.Setup("another one"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second setup = %v, want ErrAlreadyExists", err)
	}

	if _, err := m.Login("1.2.3.4", "admin", "wrong password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password = %v, want ErrInvalidCredentials", err)
	}
	token, err := m.Login("1.2.3.4", "admin", "correct horse")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	id, err := m.VerifySession(token)
	if err != nil || !id.Admin || id.Username != "admin" || id.UserID == 0 {
		t.Fatalf("verify session: id=%+v err=%v", id, err)
	}
}

func TestMultiAccountLifecycle(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}

	u, err := m.CreateUser("alice", "alice password", false, "docs")
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.Login("1.2.3.4", "alice", "alice password")
	if err != nil {
		t.Fatal(err)
	}
	id, err := m.VerifySession(token)
	if err != nil || id.Admin || id.Scope != "docs" || id.UserID != u.ID {
		t.Fatalf("scoped identity = %+v err=%v", id, err)
	}
}

func TestDuplicateUsernamesRejected(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateUser("alice", "alice password", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateUser("alice", "whatever pw", false, ""); !errors.Is(err, store.ErrExists) {
		t.Fatalf("duplicate username = %v, want ErrExists", err)
	}
}

func TestAdminGuardrails(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	users, err := m.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	var adminID int64
	for _, usr := range users {
		if usr.Admin {
			adminID = usr.ID
		}
	}
	if _, err := m.UpdateUser(adminID, false, ""); !errors.Is(err, store.ErrLastAdmin) {
		t.Fatalf("demote last admin = %v, want ErrLastAdmin", err)
	}
	if err := m.DeleteUser(adminID); !errors.Is(err, store.ErrOriginal) {
		t.Fatalf("delete original = %v, want ErrOriginal", err)
	}
}

func TestDeletionKillsSessions(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	u, err := m.CreateUser("gone", "gone password", false, "")
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.Login("1.2.3.4", "gone", "gone password")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifySession(token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("deleted user session = %v, want ErrInvalidCredentials", err)
	}
}

func TestSelfServicePasswordChange(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	u, err := m.CreateUser("pwchange", "old password", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ChangePassword(u.ID, "wrong", "whatever pw"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("change with wrong current = %v", err)
	}
	if err := m.ChangePassword(u.ID, "old password", "new password"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, err := m.Login("1.2.3.4", "pwchange", "new password"); err != nil {
		t.Fatalf("login with changed password: %v", err)
	}
}

func TestAdminSetPassword(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	u, err := m.CreateUser("bob", "bob password", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.AdminSetPassword(u.ID, "reset password"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Login("1.2.3.4", "bob", "reset password"); err != nil {
		t.Fatalf("login with reset password: %v", err)
	}
}

func TestSessionTamperAndExpiry(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	token, err := m.Login("1.2.3.4", "admin", "correct horse")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.VerifySession(token + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
	if _, err := m.VerifySession("not-a-token"); err == nil {
		t.Fatal("garbage token accepted")
	}
	// Cross-manager tokens must be rejected (different random key).
	other := newTestManager(t)
	if err := other.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	foreign, err := other.Login("1.2.3.4", "admin", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifySession(foreign); err == nil {
		t.Fatal("foreign token accepted")
	}
}

func TestSessionSurvivesRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := newTestStore(t, dir)
	m, err := New(dir, "", "seeded password", st, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	token, err := m.Login("1.2.3.4", "admin", "seeded password")
	if err != nil {
		t.Fatal(err)
	}

	// Restart against the same database and state dir, no env password.
	m2, err := New(dir, "", "", st, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if !m2.Initialized() {
		t.Fatal("account lost after restart")
	}
	if _, err := m2.VerifySession(token); err != nil {
		t.Fatalf("session invalidated by restart: %v", err)
	}
	if _, err := m2.Login("1.2.3.4", "admin", "seeded password"); err != nil {
		t.Fatalf("password lost after restart: %v", err)
	}
}

func TestSeedPasswordResetsOnRestart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	st := newTestStore(t, dir)
	if _, err := New(dir, "", "old password", st, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	m2, err := New(dir, "", "brand new pass", st, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m2.Login("1.2.3.4", "admin", "old password"); err == nil {
		t.Fatal("old seed password still valid")
	}
	if _, err := m2.Login("1.2.3.4", "admin", "brand new pass"); err != nil {
		t.Fatalf("new seed password rejected: %v", err)
	}
}

func TestSeedPasswordTooShort(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if _, err := New(dir, "", "short", newTestStore(t, dir), slog.New(slog.DiscardHandler)); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short seed = %v, want ErrWeakPassword", err)
	}
}

func TestLoginBackoff(t *testing.T) {
	t.Parallel()
	m := newTestManager(t)
	if err := m.Setup("correct horse"); err != nil {
		t.Fatal(err)
	}
	for i := range maxFails {
		if _, err := m.Login("9.9.9.9", "admin", "wrong password "+strings.Repeat("x", i)); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("attempt %d = %v", i, err)
		}
	}
	if _, err := m.Login("9.9.9.9", "admin", "correct horse"); !errors.Is(err, ErrLocked) {
		t.Fatalf("locked IP login = %v, want ErrLocked", err)
	}
	// A different IP is unaffected.
	if _, err := m.Login("8.8.8.8", "admin", "correct horse"); err != nil {
		t.Fatalf("other IP login: %v", err)
	}
	// Successful login clears backoff.
	clearIP := "7.7.7.7"
	for range maxFails {
		_, _ = m.Login(clearIP, "admin", "wrong password")
	}
	if _, err := m.Login(clearIP, "admin", "correct horse"); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
	m.clearFails(clearIP)
	if _, err := m.Login(clearIP, "admin", "correct horse"); err != nil {
		t.Fatalf("after clear: %v", err)
	}
}

func TestSessionKeyPermissions(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not representable on Windows")
	}
	dir := t.TempDir()
	if _, err := New(dir, "", "", newTestStore(t, dir), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dir, "session.key"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("session.key perm = %o, want 600", perm)
	}
}

func TestClientIP(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"192.168.1.5:1234", "192.168.1.5"},
		{"[::1]:8080", "::1"},
		{"10.0.0.1", "10.0.0.1"},
		{"garbage", ""},
	}
	for _, c := range cases {
		got := ClientIP(c.in)
		if c.want == "" {
			if got.IsValid() {
				t.Errorf("ClientIP(%q) = %q, want invalid", c.in, got)
			}
			continue
		}
		if !got.IsValid() || got.String() != c.want {
			t.Errorf("ClientIP(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
