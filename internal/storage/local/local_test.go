package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/skidoodle/filebrowser/internal/storage"
)

func newTestStorage(tb testing.TB) *Storage {
	tb.Helper()
	s, err := New(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func TestCreateListStatOpen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStorage(t)

	if _, err := s.CreateDir(ctx, "docs/sub"); err != nil {
		t.Fatalf("CreateDir: %v", err)
	}
	if _, err := s.CreateFile(ctx, "docs/sub/readme.md"); err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	// duplicate create must fail
	if _, err := s.CreateFile(ctx, "docs/sub/readme.md"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("duplicate CreateFile: want ErrExist, got %v", err)
	}

	listing, err := s.List(ctx, "docs", storage.SortOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if listing.NumDirs != 1 || listing.NumFiles != 0 {
		t.Fatalf("List: numDirs=%d numFiles=%d, want 1/0", listing.NumDirs, listing.NumFiles)
	}

	rc, info, err := s.Open(ctx, "docs/sub/readme.md")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	if info.Type != storage.TypeText {
		t.Fatalf("Open: type = %q, want text", info.Type)
	}
}

func TestPathEscapeRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStorage(t)

	for _, p := range []string{"../escape", "a/../../b", "/etc/passwd", `a\b`} {
		if _, err := s.Stat(ctx, p); err == nil {
			t.Errorf("Stat(%q): expected error", p)
		}
	}
}

func TestSymlinkEscapeRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docs", "leak")); err != nil {
		t.Skipf("symlinks unavailable on this platform/user: %v", err)
	}

	if _, _, err := s.Open(ctx, "docs/leak/secret.txt"); err == nil {
		t.Fatal("Open via escaping symlink: expected error")
	} else if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Open via escaping symlink: want ErrPermission, got %v", err)
	}
	if _, err := s.Stat(ctx, "docs/leak/secret.txt"); err == nil {
		t.Fatal("Stat via escaping symlink: expected error")
	}
}

func TestSymlinkWithinRootAllowed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real", "a.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	rc, _, err := s.Open(ctx, "link/a.txt")
	if err != nil {
		t.Fatalf("Open via in-root symlink: %v", err)
	}
	_ = rc.Close()
}

func TestUploadLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStorage(t)

	up, err := s.StartUpload(ctx, "incoming/blob.bin", 10)
	if err != nil {
		t.Fatalf("StartUpload: %v", err)
	}
	if n, err := up.Offset(); err != nil || n != 0 {
		t.Fatalf("Offset = %d, %v; want 0, nil", n, err)
	}
	if _, err := up.WriteAt([]byte("helloworld"), 0); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if n, err := up.Offset(); err != nil || n != 10 {
		t.Fatalf("Offset = %d, %v; want 10, nil", n, err)
	}
	if err := up.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	rc, info, err := s.Open(ctx, "incoming/blob.bin")
	if err != nil {
		t.Fatalf("Open after commit: %v", err)
	}
	defer rc.Close()
	if info.Size != 10 {
		t.Fatalf("size = %d, want 10", info.Size)
	}

	// Abort removes the partial file.
	up2, err := s.StartUpload(ctx, "incoming/gone.bin", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := up2.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if _, err := s.Stat(ctx, "incoming/gone.bin"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat after abort: want ErrNotExist, got %v", err)
	}
}

func TestWalk(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStorage(t)
	for _, p := range []string{"a/1.txt", "a/b/2.go", "a/b/c/3.txt"} {
		if _, err := s.CreateFile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	var got []string
	err := s.Walk(ctx, ".", storage.SearchOptions{Extension: "txt"}, func(fi storage.FileInfo) error {
		got = append(got, fi.Path)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Walk found %v, want 2 .txt files", got)
	}

	got = nil
	_ = s.Walk(ctx, ".", storage.SearchOptions{Term: "2"}, func(fi storage.FileInfo) error {
		got = append(got, fi.Path)
		return nil
	})
	if len(got) != 1 || got[0] != "a/b/2.go" {
		t.Fatalf("term search got %v", got)
	}
}

func TestUsage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStorage(t)
	u, err := s.Usage(ctx)
	if err != nil {
		t.Skipf("usage not available on this volume: %v", err)
	}
	if u.Total == 0 {
		t.Fatal("usage total is 0")
	}
	if u.Used > u.Total {
		t.Fatalf("used %d > total %d", u.Used, u.Total)
	}
}

func walkPaths(t *testing.T, s *Storage, opts storage.SearchOptions, filesOnly bool) []string {
	t.Helper()
	var got []string
	err := s.Walk(context.Background(), ".", opts, func(fi storage.FileInfo) error {
		if !filesOnly || !fi.IsDir {
			got = append(got, fi.Path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return got
}

func TestWalkNoiseDirPruning(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newTestStorage(t)

	for _, p := range []string{
		"myproject/index.js",
		"myproject/node_modules/pkg/index.js",
		"myproject/.git/objects/blob.txt",
		"photos/family.jpg",
		"photos/@eaDir/family.jpg/thumb.jpg",
	} {
		if _, err := s.CreateFile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("prune node_modules", func(t *testing.T) {
		t.Parallel()
		got := walkPaths(t, s, storage.SearchOptions{Term: "index"}, false)
		if len(got) != 1 || got[0] != "myproject/index.js" {
			t.Fatalf("search 'index' should prune node_modules, got %v", got)
		}
	})

	t.Run("prune eaDir", func(t *testing.T) {
		t.Parallel()
		got := walkPaths(t, s, storage.SearchOptions{Term: "family"}, false)
		if len(got) != 1 || got[0] != "photos/family.jpg" {
			t.Fatalf("search 'family' should prune @eaDir, got %v", got)
		}
	})

	t.Run("explicit search not pruned", func(t *testing.T) {
		t.Parallel()
		got := walkPaths(t, s, storage.SearchOptions{Term: "node_modules"}, false)
		if len(got) == 0 {
			t.Fatal("explicit search for 'node_modules' should find entries")
		}
	})

	t.Run("non-search walk includes all", func(t *testing.T) {
		t.Parallel()
		got := walkPaths(t, s, storage.SearchOptions{}, true)
		if len(got) != 5 {
			t.Fatalf("unfiltered walk should include all 5 files, got %d: %v", len(got), got)
		}
	})
}

func BenchmarkWalk(b *testing.B) {
	ctx := context.Background()
	s := newTestStorage(b)

	for d := range 30 {
		dir := fmt.Sprintf("dir_%02d", d)
		for f := range 30 {
			p := fmt.Sprintf("%s/file_%03d.txt", dir, f)
			if _, err := s.CreateFile(ctx, p); err != nil {
				b.Fatal(err)
			}
		}
	}
	if _, err := s.CreateFile(ctx, "dir_15/needle_target.txt"); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		count := 0
		err := s.Walk(ctx, ".", storage.SearchOptions{Term: "needle"}, func(fi storage.FileInfo) error {
			count++
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
		if count != 1 {
			b.Fatalf("want 1 match, got %d", count)
		}
	}
}
