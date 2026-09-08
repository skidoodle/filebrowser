package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSchemaAndUserCRUD(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	if n, err := s.CountUsers(); err != nil || n != 0 {
		t.Fatalf("fresh store users = %d err=%v", n, err)
	}
	u, err := s.CreateUser("alice", []byte("hash-alice"), false, "docs")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.UserByName("alice")
	if err != nil || got.ID != u.ID || got.Scope != "docs" || got.Admin {
		t.Fatalf("UserByName = %+v err=%v", got, err)
	}
	if _, err := s.User(u.ID); err != nil {
		t.Fatalf("User: %v", err)
	}
}

func TestUniqueUsernames(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if _, err := s.CreateUser("alice", []byte("h"), false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", []byte("h"), true, ""); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate = %v, want ErrExists", err)
	}
}

func TestInvalidValuesRejected(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	if _, err := s.CreateUser("bad name!", []byte("h"), false, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad username = %v, want ErrInvalid", err)
	}
	if _, err := s.CreateUser("ok", []byte("h"), false, "../escape"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad scope = %v, want ErrInvalid", err)
	}
	if _, err := s.CreateUser("ok", []byte("h"), false, "a/../b"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad scope = %v, want ErrInvalid", err)
	}
}

func TestRenameUser(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	u, err := s.CreateUser("alice", []byte("h"), false, "docs")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.RenameUser(u.ID, "alicia")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got.Username != "alicia" || got.Scope != "docs" {
		t.Fatalf("renamed = %+v", got)
	}
	// Old name gone, new name resolvable.
	if _, err := s.UserByName("alice"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old name still resolvable: %v", err)
	}
	// Duplicate target rejected.
	other, err := s.CreateUser("bob", []byte("h"), false, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameUser(other.ID, "alicia"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename to taken name = %v, want ErrExists", err)
	}
	// Invalid names rejected.
	if _, err := s.RenameUser(u.ID, "bad name!"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("rename to invalid = %v, want ErrInvalid", err)
	}
	// Same name is a no-op success.
	if _, err := s.RenameUser(u.ID, "alicia"); err != nil {
		t.Fatalf("rename to same name: %v", err)
	}
}

func TestUserUpdateAndList(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	u, err := s.CreateUser("alice", []byte("h"), false, "docs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateUser(u.ID, true, ""); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := s.User(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Admin || got.Scope != "" {
		t.Fatalf("after update = %+v", got)
	}
	users, err := s.ListUsers()
	if err != nil || len(users) != 1 {
		t.Fatalf("list = %+v err=%v", users, err)
	}
}

func TestCreateOriginalIdempotent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	id1, err := s.CreateOriginal([]byte("hash-1"))
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.CreateOriginal([]byte("hash-2"))
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("original ids differ: %d vs %d", id1, id2)
	}
	u, err := s.UserByName("admin")
	if err != nil || !u.IsOriginal || !u.Admin {
		t.Fatalf("original = %+v err=%v", u, err)
	}
	if string(u.PasswordHash) != "hash-2" {
		t.Fatalf("original hash not reset: %q", u.PasswordHash)
	}
}

func TestGuardrails(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	orig, err := s.CreateOriginal([]byte("h1"))
	if err != nil {
		t.Fatal(err)
	}
	// Only one admin so far: demote and delete must both fail.
	if _, err := s.UpdateUser(orig, false, ""); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote original alone = %v, want ErrLastAdmin", err)
	}
	if err := s.DeleteUser(orig); !errors.Is(err, ErrOriginal) {
		t.Fatalf("delete original = %v, want ErrOriginal", err)
	}

	second, err := s.CreateUser("second", []byte("h2"), true, "")
	if err != nil {
		t.Fatal(err)
	}
	// Now demoting the original is allowed (two admins).
	if _, err := s.UpdateUser(orig, false, ""); err != nil {
		t.Fatalf("demote with second admin: %v", err)
	}
	// But the last admin rule kicks back in if second is demoted.
	if _, err := s.UpdateUser(second.ID, false, ""); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote second = %v, want ErrLastAdmin", err)
	}
	if _, err := s.User(orig); err != nil {
		t.Fatal(err)
	}
	// Deleting the only remaining admin is refused too.
	if err := s.DeleteUser(second.ID); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("delete last admin = %v, want ErrLastAdmin", err)
	}
	// Re-promote the original, then the non-original admin can be deleted.
	if _, err := s.UpdateUser(orig, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(second.ID); err != nil {
		t.Fatalf("delete second admin: %v", err)
	}
	// Deleting a missing user is 404-shaped.
	if err := s.DeleteUser(9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing = %v, want ErrNotFound", err)
	}
}

func TestPrivateFoldersLifecycle(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	alice, err := s.CreateUser("alice", []byte("h"), false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPrivate("docs/private", alice.ID); err != nil {
		t.Fatal(err)
	}
	priv, err := s.PrivateFolders()
	if err != nil || priv["docs/private"] != alice.ID {
		t.Fatalf("private = %+v err=%v", priv, err)
	}

	// Rename rewrites the prefix.
	if err := s.SetPrivate("docs/private/nested", alice.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RenamePrivate("docs/private", "archive/kept"); err != nil {
		t.Fatal(err)
	}
	priv, _ = s.PrivateFolders()
	if _, ok := priv["archive/kept/nested"]; !ok {
		t.Fatalf("nested not re-rooted: %+v", priv)
	}
	if _, ok := priv["docs/private"]; ok {
		t.Fatalf("old path still present: %+v", priv)
	}

	// Delete below removes marks.
	if err := s.DeletePrivateBelow("archive"); err != nil {
		t.Fatal(err)
	}
	priv, _ = s.PrivateFolders()
	if len(priv) != 0 {
		t.Fatalf("marks survived delete-below: %+v", priv)
	}

	// Invalid paths are rejected.
	if err := s.SetPrivate("../evil", alice.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad private path = %v, want ErrInvalid", err)
	}
	if err := s.SetPrivate(".", alice.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("root private path = %v, want ErrInvalid", err)
	}
}

func TestUserDeletionCascadesPrivateFolders(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	alice, err := s.CreateUser("alice", []byte("h"), false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPrivate("secret", alice.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(alice.ID); err != nil {
		t.Fatal(err)
	}
	priv, err := s.PrivateFolders()
	if err != nil || len(priv) != 0 {
		t.Fatalf("private marks survived cascade: %+v err=%v", priv, err)
	}
}
