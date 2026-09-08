package fsutil

import (
	"errors"
	"os"
	"testing"

	"github.com/skidoodle/filebrowser/internal/storage"
)

func TestClean(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", ".", false},
		{"a/b/c.txt", "a/b/c.txt", false},
		{"a//b", "a/b", false},
		{"./a", "a", false},
		{".", ".", false},
		{"/abs", "", true},
		{"../escape", "", true},
		{"a/../..", "", true},
		{"a/../../b", "", true},
		{"back\\slash", "", true},
		{"nul\x00byte", "", true},
		{string(make([]byte, 300)), "", true}, // segment too long
	}
	for _, tc := range cases {
		got, err := Clean(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("Clean(%q) = %q, want error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Clean(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanRejectsEscapeWithErrInvalid(t *testing.T) {
	t.Parallel()
	if _, err := Clean("../escape"); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("want os.ErrInvalid, got %v", err)
	}
}

const (
	file9  = "file9"
	file10 = "file10"
)

func TestNaturalCompare(t *testing.T) {
	t.Parallel()
	less := [][2]string{
		{"a", "b"},
		{"file2", file10},
		{"file02", file10},
		{"img1", "img2a"},
		{"a1b", "a1c"},
		{"File10", file9},
	}
	for _, c := range less {
		if NaturalCompare(c[0], c[1]) >= 0 {
			t.Errorf("NaturalCompare(%q, %q) should be < 0", c[0], c[1])
		}
		if NaturalCompare(c[1], c[0]) <= 0 {
			t.Errorf("NaturalCompare(%q, %q) should be > 0", c[1], c[0])
		}
	}
	if NaturalCompare("same", "same") != 0 {
		t.Error("equal strings should compare 0")
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		head []byte
		want storage.FileType
	}{
		{"video.mp4", nil, storage.TypeVideo},
		{"song.FLAC", nil, storage.TypeAudio},
		{"photo.jpeg", nil, storage.TypeImage},
		{"doc.pdf", nil, storage.TypePDF},
		{"vector.ai", nil, storage.TypePDF},
		{"audio.mka", nil, storage.TypeAudio},
		{"audio.ac3", nil, storage.TypeAudio},
		{"main.go", nil, storage.TypeText},
		{"trace.har", nil, storage.TypeText},
		{"map.geojson", nil, storage.TypeText},
		{"places.kml", nil, storage.TypeText},
		{"Dockerfile", nil, storage.TypeText},
		{"notes.txt", nil, storage.TypeText},
		{"empty.txt", nil, storage.TypeText}, // no content to sniff
		{"console.log", nil, storage.TypeText},
		{"README", nil, storage.TypeText},  // base-name match, no extension
		{"LICENSE", nil, storage.TypeText}, // base-name match, no extension
		{"unknown.xyz", []byte("hello world"), storage.TypeText},
		{"unknown.bin", []byte{0x00, 0x01, 0x02}, storage.TypeBlob},
	}
	for _, tc := range cases {
		got, _ := Detect(tc.name, tc.head)
		if got != tc.want {
			t.Errorf("Detect(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestDetectScriptMime pins the .js-family MIME types: on Windows the mime
// package inherits text/plain from the registry, which breaks strict MIME
// checking for module scripts (spa assets, pdf.js workers).
func TestDetectScriptMime(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"app.js", "app.mjs", "worker.cjs"} {
		if _, mime := Detect(name, nil); mime != "text/javascript" {
			t.Errorf("Detect(%q) mime = %q, want text/javascript", name, mime)
		}
	}
}

func TestSortFileInfos(t *testing.T) {
	t.Parallel()
	items := []storage.FileInfo{
		{Name: "b.txt", Size: 3, IsDir: false},
		{Name: "zdir", IsDir: true},
		{Name: file10, Size: 1},
		{Name: file9, Size: 1},
		{Name: "adir", IsDir: true},
		{Name: "a.txt", Size: 2},
	}

	SortFileInfos(items, storage.SortOptions{By: "name", Asc: true})
	want := []string{"adir", "zdir", "a.txt", "b.txt", file9, file10}
	for i, w := range want {
		if items[i].Name != w {
			t.Fatalf("asc: items[%d] = %q, want %q (full: %v)", i, items[i].Name, w, names(items))
		}
	}

	SortFileInfos(items, storage.SortOptions{By: "name", Asc: false})
	want = []string{"zdir", "adir", file10, file9, "b.txt", "a.txt"}
	for i, w := range want {
		if items[i].Name != w {
			t.Fatalf("desc: items[%d] = %q, want %q (full: %v)", i, items[i].Name, w, names(items))
		}
	}
}

func names(items []storage.FileInfo) []string {
	out := make([]string, len(items))
	for i, fi := range items {
		out[i] = fi.Name
	}
	return out
}
