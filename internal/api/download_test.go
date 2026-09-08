package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDownloadZipWholeDir(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	ctx := t.Context()

	up, _ := store.StartUpload(ctx, "docs/a.txt", 0)
	if _, err := up.WriteAt([]byte("alpha"), 0); err != nil {
		t.Fatal(err)
	}
	if err := up.Commit(); err != nil {
		t.Fatal(err)
	}
	up2, _ := store.StartUpload(ctx, "docs/sub/b.txt", 0)
	if _, err := up2.WriteAt([]byte("beta!"), 0); err != nil {
		t.Fatal(err)
	}
	if err := up2.Commit(); err != nil {
		t.Fatal(err)
	}

	res := testGet(t, ts, ts.URL+"/api/download?path=docs&algo=zip")
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "docs.zip") {
		t.Fatalf("disposition = %q", cd)
	}

	data := mustRead(t, res.Body)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip reader: %v", err)
	}
	found := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(rc)
		_ = rc.Close()
		found[f.Name] = string(content)
	}
	if found["a.txt"] != "alpha" || found["sub/b.txt"] != "beta!" {
		t.Fatalf("zip contents: %#v", found)
	}
}

func mustRead(t *testing.T, r io.Reader) []byte {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDownloadZipSelection(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	ctx := t.Context()

	for _, p := range []string{"d/one.txt", "d/two.txt", "d/three.txt"} {
		up, err := store.StartUpload(ctx, p, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = up.WriteAt([]byte(p), 0)
		if err := up.Commit(); err != nil {
			t.Fatal(err)
		}
	}

	res := testGet(t, ts, ts.URL+"/api/download?path=d&files=one.txt,three.txt&algo=zip")
	defer func() { _ = res.Body.Close() }()
	data := mustRead(t, res.Body)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("zip: %v", err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(rc)
		_ = rc.Close()
		got[f.Name] = string(content)
	}
	if len(got) != 2 || got["one.txt"] != "d/one.txt" || got["three.txt"] != "d/three.txt" {
		t.Fatalf("zip contents: %#v", got)
	}
}

func TestDownloadTarGz(t *testing.T) {
	t.Parallel()
	ts, store := newTestServer(t, 1<<20)
	ctx := t.Context()

	up, _ := store.StartUpload(ctx, "logs/app.log", 0)
	if _, err := up.WriteAt([]byte("line one\nline two\n"), 0); err != nil {
		t.Fatal(err)
	}
	if err := up.Commit(); err != nil {
		t.Fatal(err)
	}

	res := testGet(t, ts, ts.URL+"/api/download?path=logs&algo=tar.gz")
	defer func() { _ = res.Body.Close() }()

	gz, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar: %v", err)
	}
	if hdr.Name != "app.log" {
		t.Fatalf("entry name = %q", hdr.Name)
	}
	content, _ := io.ReadAll(tr)
	if string(content) != "line one\nline two\n" {
		t.Fatalf("content = %q", content)
	}
	if _, err := tr.Next(); err == nil {
		t.Fatal("expected end of archive")
	}
}

func TestDownloadRejectsBadSelection(t *testing.T) {
	t.Parallel()
	ts, _ := newTestServer(t, 1<<20)
	res := testGet(t, ts, ts.URL+"/api/download?path=d&files=../escape.txt")
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}
