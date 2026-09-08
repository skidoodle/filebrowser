package local

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skidoodle/filebrowser/internal/storage"
)

func TestMove(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)

	if _, err := s.CreateDir(t.Context(), "dst"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFile(t.Context(), "src.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(t.Context(), "src.txt", strings.NewReader("payload"), 16); err != nil {
		t.Fatal(err)
	}

	info, err := s.Move(t.Context(), "src.txt", "dst/renamed.txt")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if info.Path != "dst/renamed.txt" {
		t.Fatalf("moved path = %q", info.Path)
	}
	if _, err := s.Stat(t.Context(), "src.txt"); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	rc, _, err := s.Open(t.Context(), "dst/renamed.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "payload" {
		t.Fatalf("content = %q", data)
	}
}

func TestMoveConflicts(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	_, err := s.CreateFile(t.Context(), "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateFile(t.Context(), "b.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDir(t.Context(), "dir"); err != nil {
		t.Fatal(err)
	}

	// Destination exists.
	if _, err := s.Move(t.Context(), "a.txt", "b.txt"); !os.IsExist(err) {
		t.Fatalf("move onto existing = %v, want ErrExist", err)
	}
	// Directory into its own subtree.
	if _, err := s.Move(t.Context(), "dir", "dir/sub"); err == nil {
		t.Fatal("move dir into itself succeeded")
	}
	// Escaping paths.
	if _, err := s.Move(t.Context(), "a.txt", "../escape"); err == nil {
		t.Fatal("escape destination accepted")
	}
	if _, err := s.Move(t.Context(), "../escape", "a.txt"); err == nil {
		t.Fatal("escape source accepted")
	}
}

func TestWriteOverwriteAndCreate(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)

	if _, err := s.Write(t.Context(), "new.txt", strings.NewReader("v1"), 16); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.Write(t.Context(), "new.txt", strings.NewReader("longer v2 content"), 32); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	rc, fi, err := s.Open(t.Context(), "new.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "longer v2 content" || fi.Size != int64(len(data)) {
		t.Fatalf("content = %q size = %d", data, fi.Size)
	}
}

func TestWriteSizeCap(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	if _, err := s.Write(t.Context(), "capped.txt", bytes.NewReader(bytes.Repeat([]byte("x"), 100)), 10); err != nil {
		t.Fatalf("write: %v", err)
	}
	rc, fi, err := s.Open(t.Context(), "capped.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if fi.Size != 10 {
		t.Fatalf("size = %d, want 10 (cap enforced)", fi.Size)
	}
}

func TestReservedNamespaceInvisible(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)

	resDir := filepath.Join(s.root, reservedDir)
	if err := os.MkdirAll(resDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resDir, "admin.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFile(t.Context(), "visible.txt"); err != nil {
		t.Fatal(err)
	}

	// Not listable.
	listing, err := s.List(t.Context(), ".", storage.SortOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range listing.Items {
		if item.Name == reservedDir {
			t.Fatal("reserved dir appears in listing")
		}
	}

	// Not statable/openable/removable/movable/writable through the port.
	for name, fn := range map[string]func() error{
		"stat":    func() error { _, err := s.Stat(t.Context(), reservedDir+"/admin.json"); return err },
		"open":    func() error { _, _, err := s.Open(t.Context(), reservedDir+"/admin.json"); return err },
		"remove":  func() error { return s.Remove(t.Context(), reservedDir) },
		"moveTo":  func() error { _, err := s.Move(t.Context(), "visible.txt", reservedDir+"/x"); return err },
		"moveFr":  func() error { _, err := s.Move(t.Context(), reservedDir+"/admin.json", "x"); return err },
		"write":   func() error { _, err := s.Write(t.Context(), reservedDir+"/x", strings.NewReader("y"), 1); return err },
		"create":  func() error { _, err := s.CreateFile(t.Context(), reservedDir+"/x"); return err },
		"mkdir":   func() error { _, err := s.CreateDir(t.Context(), reservedDir+"/sub"); return err },
		"upload":  func() error { _, err := s.StartUpload(t.Context(), reservedDir+"/x", 1); return err },
		"subpath": func() error { _, err := s.Stat(t.Context(), "sub/"+reservedDir+"/x"); return err },
	} {
		if err := fn(); err == nil {
			t.Errorf("reserved namespace operation %q succeeded", name)
		}
	}

	// Not searchable.
	found := false
	err = s.Walk(t.Context(), ".", storage.SearchOptions{}, func(fi storage.FileInfo) error {
		if strings.Contains(fi.Path, reservedDir) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("reserved namespace appears in walk")
	}

	// But still present on disk (durability).
	if _, err := os.Stat(filepath.Join(resDir, "admin.json")); err != nil {
		t.Fatalf("reserved file vanished: %v", err)
	}
}

func TestMoveDirAcrossTree(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t)
	if _, err := s.CreateDir(t.Context(), "a/b"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFile(t.Context(), "a/b/f.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateDir(t.Context(), "c"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Move(t.Context(), "a/b", "c/b"); err != nil {
		t.Fatalf("move subtree: %v", err)
	}
	if _, err := s.Stat(t.Context(), "c/b/f.txt"); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}
}
