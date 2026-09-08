package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"
)

// FileType classifies entries for the frontend viewers.
type FileType string

// File type values as they appear in JSON listings.
const (
	TypeDir   FileType = "dir"
	TypeVideo FileType = "video"
	TypeAudio FileType = "audio"
	TypeImage FileType = "image"
	TypePDF   FileType = "pdf"
	TypeText  FileType = "text"
	TypeBlob  FileType = "blob"
)

// FileInfo describes a single file or directory.
type FileInfo struct {
	Name      string      `json:"name"`
	Path      string      `json:"path"` // slash-separated path relative to the storage root
	Size      int64       `json:"size"`
	ModTime   time.Time   `json:"modified"`
	Mode      fs.FileMode `json:"-"`
	IsDir     bool        `json:"isDir"`
	Type      FileType    `json:"type"`
	MimeType  string      `json:"mime,omitempty"`
	Extension string      `json:"extension,omitempty"`
	// Private is annotated by the API layer (never by adapters): the entry
	// is a directory the viewer (or an admin) marked private.
	Private bool `json:"private,omitempty"`
}

// Usage reports consumed and total capacity of the backing store.
type Usage struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// SortOptions controls listing order. Dirs always sort before files.
type SortOptions struct {
	By  string // "name", "size", "modified"
	Asc bool
}

// Listing is the result of listing a directory.
type Listing struct {
	Items    []FileInfo `json:"items"`
	NumDirs  int        `json:"numDirs"`
	NumFiles int        `json:"numFiles"`
	Total    int        `json:"total"`
}

// SearchOptions narrows a Walk.
type SearchOptions struct {
	// CaseInsensitive substring the path must contain; empty matches all.
	Term string
	// Type filter, e.g. storage.TypeImage; empty matches all.
	Type FileType
	// Extension filter without dot, e.g. "go"; empty matches all.
	Extension string
	// Limit caps the number of results; 0 means unlimited.
	Limit int
}

// Upload is an in-progress chunked upload. Offsets are absolute byte
// positions; Commit finalizes and Abort discards the partial data.
type Upload interface {
	// Offset reports how many bytes have been written so far (tus Upload-Offset).
	Offset() (int64, error)
	// WriteAt writes p at the given absolute offset.
	WriteAt(p []byte, off int64) (int, error)
	// Commit flushes all pending data and closes the upload.
	Commit() error
	// Abort discards the partial upload.
	Abort() error
}

// ErrDone is returned by Walk visitors to stop iteration early; Walk
// absorbs it and reports success.
var ErrDone = errors.New("storage: done")

// Storage is the port every backend must implement. Paths are
// slash-separated and relative to the backend root, never absolute,
// never containing "..". Implementations must confine all access to
// the root and reject any escaping path.
type Storage interface {
	// Stat returns metadata for path.
	Stat(ctx context.Context, path string) (FileInfo, error)
	// List returns the sorted contents of a directory.
	List(ctx context.Context, path string, sort SortOptions) (Listing, error)
	// Open opens path for reading. The returned reader supports Seeking
	// so downloads can honor HTTP Range requests.
	Open(ctx context.Context, path string) (io.ReadSeekCloser, FileInfo, error)
	// CreateDir creates path and any missing parents. It is a no-op if
	// path already exists as a directory.
	CreateDir(ctx context.Context, path string) (FileInfo, error)
	// CreateFile creates an empty file, failing if it already exists.
	CreateFile(ctx context.Context, path string) (FileInfo, error)
	// Usage reports used and total bytes of the backing volume.
	Usage(ctx context.Context) (Usage, error)
	// Walk visits every entry below root, depth-first. If fn returns an
	// error the walk stops and returns it; context cancellation aborts.
	Walk(ctx context.Context, root string, opts SearchOptions, fn func(FileInfo) error) error
	// StartUpload begins a chunked upload writing to path, truncating any
	// existing partial at that path. The declared size is advisory for the
	// caller (tus Upload-Length enforcement) but backends may preallocate.
	StartUpload(ctx context.Context, path string, size int64) (Upload, error)
	// Move renames from to to, failing if the destination exists. Both
	// paths must resolve inside the root. Admin-only.
	Move(ctx context.Context, from, to string) (FileInfo, error)
	// Write replaces or creates the file at path with the contents of r.
	// size caps how much of r is consumed. The write is atomic: readers
	// never observe a partial file. Admin-only.
	Write(ctx context.Context, path string, r io.Reader, size int64) (FileInfo, error)
	// Remove deletes path (including non-empty directories). It backs
	// internal maintenance such as abandoned-upload GC; delete endpoints
	// must expose it only to authenticated administrators.
	Remove(ctx context.Context, path string) error
}
