package authz

import (
	"errors"
	"testing"

	"github.com/skidoodle/filebrowser/internal/store"
)

// memSource is an in-memory PrivateSource for permission-matrix tests.
type memSource struct {
	priv map[string]int64
}

func newMemSource() *memSource { return &memSource{priv: map[string]int64{}} }

func (m *memSource) PrivateFolders() (map[string]int64, error) { return m.priv, nil }

func (m *memSource) SetPrivate(dir string, ownerID int64) error {
	m.priv[dir] = ownerID
	return nil
}

func (m *memSource) UnsetPrivate(dir string) error {
	delete(m.priv, dir)
	return nil
}

func (m *memSource) RenamePrivate(from, to string) error {
	out := map[string]int64{}
	for p, owner := range m.priv {
		switch {
		case p == from:
			out[to] = owner
		case len(p) > len(from)+1 && p[:len(from)+1] == from+"/":
			out[to+p[len(from):]] = owner
		default:
			out[p] = owner
		}
	}
	m.priv = out
	return nil
}

func (m *memSource) DeletePrivateBelow(dir string) error {
	out := map[string]int64{}
	for p, owner := range m.priv {
		if p == dir || len(p) > len(dir)+1 && p[:len(dir)+1] == dir+"/" {
			continue
		}
		out[p] = owner
	}
	m.priv = out
	return nil
}

func newTestEngine(t *testing.T) (*Engine, *memSource) {
	t.Helper()
	src := newMemSource()
	e, err := New(src)
	if err != nil {
		t.Fatal(err)
	}
	return e, src
}

const (
	alicePrivate = "alice/private"
	aliceNested  = "alice/private/inner.txt"
	alicePublic  = "alice/public"
)

var (
	admin  = Actor{UserID: 1, Admin: true}
	alice  = Actor{UserID: 2, Scope: "alice"}
	bob    = Actor{UserID: 3, Scope: "bob"}
	guest  = Actor{}
	alice2 = Actor{UserID: 4, Scope: "alice"} // shares alice's scope
)

func TestScopeBoundaryWriteMatrix(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)

	cases := []struct {
		actor Actor
		path  string
		want  error
	}{
		{admin, "anything/here", nil},
		{admin, ".", nil},
		{alice, "alice/file.txt", nil},
		{alice, "alice/deep/nested/file", nil},
		{alice, "alice", ErrOutOfScope}, // the scope folder itself is off-limits
		{alice, "bob/file.txt", ErrOutOfScope},
		{alice, "alicia/lookalike", ErrOutOfScope},
		{alice, "bobby", ErrOutOfScope},
		{bob, "alice/x", ErrOutOfScope},
		{guest, "alice/x", ErrDenied},
		{guest, "x", ErrDenied},
	}
	for _, c := range cases {
		err := e.CanWrite(c.actor, c.path)
		if !errors.Is(err, c.want) {
			t.Errorf("CanWrite(%+v, %q) = %v, want %v", c.actor, c.path, err, c.want)
		}
	}
}

func TestPrivateVisibilityMatrix(t *testing.T) {
	t.Parallel()
	e, src := newTestEngine(t)
	if err := e.SetPrivate(alice, alicePrivate); err != nil {
		t.Fatal(err)
	}
	_ = src

	// Reads: hidden from guests, other users, and even scope-sharers;
	// visible to owner and admin. Hidden = ErrPrivate → 404 semantics.
	cases := []struct {
		actor Actor
		path  string
		want  error
	}{
		{alice, alicePrivate, nil},
		{alice, aliceNested, nil},
		{alice, alicePublic, nil},
		{admin, alicePrivate, nil},
		{bob, alicePrivate, ErrPrivate},
		{bob, aliceNested, ErrPrivate},
		{guest, alicePrivate, ErrPrivate},
		{alice2, alicePrivate, ErrPrivate},
		// Siblings are unaffected.
		{bob, alicePublic, nil},
		{guest, "alice/other", nil},
	}
	for _, c := range cases {
		err := e.CanRead(c.actor, c.path)
		if !errors.Is(err, c.want) {
			t.Errorf("CanRead(%+v, %q) = %v, want %v", c.actor, c.path, err, c.want)
		}
	}

	// Writes inside someone else's private folder are blocked even in-scope.
	if err := e.CanWrite(alice2, "alice/private/x"); !errors.Is(err, ErrPrivate) {
		t.Errorf("write into foreign private = %v, want ErrPrivate", err)
	}
	if err := e.CanWrite(admin, "alice/private/x"); err != nil {
		t.Errorf("admin write into private = %v, want nil", err)
	}
}

func TestFilterPaths(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)
	if err := e.SetPrivate(alice, alicePrivate); err != nil {
		t.Fatal(err)
	}
	paths := []string{"alice/public.txt", alicePrivate, "alice/private/x.txt", "bob/f.txt"}
	got := e.FilterPaths(guest, append([]string{}, paths...))
	want := []string{"alice/public.txt", "bob/f.txt"}
	if len(got) != len(want) {
		t.Fatalf("guest filter = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("guest filter = %v, want %v", got, want)
		}
	}
	// The owner still sees their own private folders (and everything else;
	// read access is guest-level everywhere).
	got = e.FilterPaths(alice, append([]string{}, paths...))
	if len(got) != len(paths) {
		t.Fatalf("owner filter = %v, want all", got)
	}
	got = e.FilterPaths(admin, append([]string{}, paths...))
	if len(got) != len(paths) {
		t.Fatalf("admin filter = %v, want all", got)
	}
}

func TestPrivateToggleRules(t *testing.T) {
	t.Parallel()
	e, _ := newTestEngine(t)

	// Scoped users can only privatize inside their scope (never the scope
	// folder itself).
	if err := e.SetPrivate(alice, "alice/secret"); err != nil {
		t.Fatalf("own-scope private: %v", err)
	}
	if err := e.SetPrivate(alice, "alice"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("scope-root private = %v, want ErrOutOfScope", err)
	}
	if err := e.SetPrivate(alice, "bob/secret"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("out-of-scope private = %v, want ErrOutOfScope", err)
	}
	if err := e.SetPrivate(alice, "."); !errors.Is(err, ErrDenied) {
		t.Fatalf("root private = %v, want ErrDenied", err)
	}
	if err := e.SetPrivate(guest, "alice/other"); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("guest private = %v, want ErrOutOfScope", err)
	}
	if err := e.SetPrivate(admin, "anywhere/secret"); err != nil {
		t.Fatalf("admin private: %v", err)
	}

	// Unset: owner or admin only.
	if err := e.UnsetPrivate(alice2, "alice/secret"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("foreign unset = %v, want ErrPrivate", err)
	}
	if err := e.UnsetPrivate(alice, "alice/secret"); err != nil {
		t.Fatalf("owner unset: %v", err)
	}
	if err := e.UnsetPrivate(admin, "anywhere/secret"); err != nil {
		t.Fatalf("admin unset: %v", err)
	}
}

func TestRenameAndDeletePrivateRewrite(t *testing.T) {
	t.Parallel()
	e, src := newTestEngine(t)
	if err := e.SetPrivate(admin, "old/priv"); err != nil {
		t.Fatal(err)
	}
	if err := e.RenamePrivate("old", "new"); err != nil {
		t.Fatal(err)
	}
	if _, ok := src.priv["new/priv"]; !ok {
		t.Fatalf("private mark not re-rooted: %+v", src.priv)
	}
	// Guests now see 404 for the new location...
	if err := e.CanRead(guest, "new/priv"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("renamed private read = %v", err)
	}
	// ...and the old one is visible again.
	if err := e.CanRead(guest, "old/priv"); err != nil {
		t.Fatalf("old path read = %v, want nil", err)
	}
	if err := e.DeletePrivateBelow("new"); err != nil {
		t.Fatal(err)
	}
	if err := e.CanRead(guest, "new/priv"); err != nil {
		t.Fatalf("after delete-below: %v, want nil", err)
	}
}

func TestStoreBackedEngine(t *testing.T) {
	t.Parallel()
	s, err := store.Open(t.TempDir() + "/users.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	u, err := s.CreateUser("alice", []byte("h"), false, "")
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetPrivate(Actor{UserID: u.ID}, "p"); err != nil {
		t.Fatal(err)
	}
	if err := e.CanRead(guest, "p"); !errors.Is(err, ErrPrivate) {
		t.Fatalf("store-backed private = %v", err)
	}
}
