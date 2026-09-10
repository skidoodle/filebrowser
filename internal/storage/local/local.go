package local

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/skidoodle/filebrowser/internal/fsutil"
	"github.com/skidoodle/filebrowser/internal/storage"
)

// reservedDir is a hidden namespace at the storage root holding durable
// server state (admin credentials, session key). It is invisible and
// inaccessible through the Storage port.
const reservedDir = ".filebrowser"

// Storage is the local filesystem adapter.
type Storage struct {
	root     string // absolute root as configured
	evalRoot string // symlink-evaluated root, used as confinement boundary
}

// compile-time check that the adapter satisfies the port.
var _ storage.Storage = (*Storage)(nil)

// New opens (and creates if missing) the storage root.
func New(root string) (*Storage, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	eval, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Storage{root: abs, evalRoot: eval}, nil
}

// resolve validates p, evaluates symlinks of its deepest existing ancestor
// and verifies the result stays inside the root. The returned absolute path
// is safe to use for filesystem operations.
func (s *Storage) resolve(p string) (string, string, error) {
	rel, err := fsutil.Clean(p)
	if err != nil {
		return "", "", err
	}
	if isReserved(rel) {
		return "", "", os.ErrPermission
	}
	abs := filepath.Join(s.root, filepath.FromSlash(rel))

	target, err := evalExisting(abs)
	if err != nil {
		return "", "", err
	}
	if !withinRoot(s.evalRoot, target) {
		return "", "", os.ErrPermission
	}
	return rel, target, nil
}

// evalExisting evaluates symlinks along the path. For missing targets the
// deepest existing ancestor is evaluated instead, so callers can still
// create new files safely.
func evalExisting(abs string) (string, error) {
	if _, err := os.Lstat(abs); err == nil {
		return filepath.EvalSymlinks(abs)
	}
	// Climb to the nearest existing ancestor.
	ancestor := filepath.Dir(abs)
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		}
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return "", fs.ErrNotExist
		}
		ancestor = next
	}
	evalAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	rest, err := filepath.Rel(ancestor, abs)
	if err != nil {
		return "", err
	}
	return filepath.Join(evalAncestor, filepath.FromSlash(rest)), nil
}

// isReserved reports whether the cleaned relative path touches the
// reserved server-state namespace.
func isReserved(rel string) bool {
	if rel == "." {
		return false
	}
	for seg := range strings.SplitSeq(rel, "/") {
		if seg == reservedDir {
			return true
		}
	}
	return false
}

// withinRoot reports whether target is the root or below it.
func withinRoot(root, target string) bool {
	if target == root {
		return true
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// info builds a FileInfo for an entry. For files without a recognizable
// extension, head (first bytes, may be nil) enables content sniffing.
func (s *Storage) info(rel string, fi fs.FileInfo, head []byte) storage.FileInfo {
	out := storage.FileInfo{
		Name:    fi.Name(),
		Path:    rel,
		Size:    fi.Size(),
		ModTime: fi.ModTime().UTC(),
		Mode:    fi.Mode(),
		IsDir:   fi.IsDir(),
	}
	if out.Path == "." {
		out.Name = "/"
	}
	if fi.IsDir() {
		out.Type = storage.TypeDir
		return out
	}
	out.Extension = strings.TrimPrefix(strings.ToLower(path.Ext(fi.Name())), ".")
	out.Type, out.MimeType = fsutil.Detect(fi.Name(), head)
	return out
}

// readHead returns the first 512 bytes of a file for content sniffing.
func readHead(abs string) []byte {
	f, err := os.Open(abs)
	if err != nil {
		return nil
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if n <= 0 {
		return nil
	}
	return head[:n]
}

// Stat returns metadata for path.
func (s *Storage) Stat(_ context.Context, p string) (storage.FileInfo, error) {
	rel, target, err := s.resolve(p)
	if err != nil {
		return storage.FileInfo{}, err
	}
	fi, err := os.Stat(target)
	if err != nil {
		return storage.FileInfo{}, err
	}
	var head []byte
	if !fi.IsDir() {
		head = readHead(target)
	}
	return s.info(rel, fi, head), nil
}

// List returns the sorted contents of a directory.
func (s *Storage) List(ctx context.Context, p string, sort storage.SortOptions) (storage.Listing, error) {
	rel, target, err := s.resolve(p)
	if err != nil {
		return storage.Listing{}, err
	}
	fi, err := os.Stat(target)
	if err != nil {
		return storage.Listing{}, err
	}
	if !fi.IsDir() {
		return storage.Listing{}, os.ErrInvalid
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		return storage.Listing{}, err
	}

	listing := storage.Listing{Items: make([]storage.FileInfo, 0, len(entries))}
	for _, e := range entries {
		if ctx.Err() != nil {
			return storage.Listing{}, ctx.Err()
		}
		if rel == "." && e.Name() == reservedDir {
			continue // hidden server-state namespace
		}
		efi, err := e.Info()
		if err != nil {
			continue // vanished between ReadDir and Info
		}
		childRel := e.Name()
		if rel != "." {
			childRel = rel + "/" + e.Name()
		}
		// Extension-only detection keeps listings cheap; unknown extensions
		// fall back to blob and get sniffed on individual Stat.
		listing.Items = append(listing.Items, s.info(childRel, efi, nil))
		if e.IsDir() {
			listing.NumDirs++
		} else {
			listing.NumFiles++
		}
	}
	listing.Total = len(listing.Items)
	fsutil.SortFileInfos(listing.Items, sort)
	return listing, nil
}

// Open opens path for reading; the reader supports Seeking so downloads
// can honor HTTP Range requests.
func (s *Storage) Open(_ context.Context, p string) (io.ReadSeekCloser, storage.FileInfo, error) {
	rel, target, err := s.resolve(p)
	if err != nil {
		return nil, storage.FileInfo{}, err
	}
	f, err := os.Open(target)
	if err != nil {
		return nil, storage.FileInfo{}, err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, storage.FileInfo{}, err
	}
	return f, s.info(rel, fi, nil), nil
}

// CreateDir creates path and any missing parents (no-op if it exists).
func (s *Storage) CreateDir(_ context.Context, p string) (storage.FileInfo, error) {
	rel, target, err := s.resolve(p)
	if err != nil {
		return storage.FileInfo{}, err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return storage.FileInfo{}, err
	}
	fi, err := os.Stat(target)
	if err != nil {
		return storage.FileInfo{}, err
	}
	return s.info(rel, fi, nil), nil
}

// CreateFile creates an empty file, failing if it already exists.
func (s *Storage) CreateFile(_ context.Context, p string) (storage.FileInfo, error) {
	rel, target, err := s.resolve(p)
	if err != nil {
		return storage.FileInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return storage.FileInfo{}, err
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return storage.FileInfo{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return storage.FileInfo{}, err
	}
	return s.info(rel, fi, nil), nil
}

// Usage reports used and total bytes of the backing volume.
func (s *Storage) Usage(ctx context.Context) (storage.Usage, error) {
	// statfs via gopsutil keeps this cross-platform (Windows included).
	return diskUsage(ctx, s.root)
}

// Walk visits every entry below p, depth-first; fn may abort with storage.ErrDone.
func (s *Storage) Walk(ctx context.Context, p string, opts storage.SearchOptions, fn func(storage.FileInfo) error) error {
	baseRel, base, err := s.resolve(p)
	if err != nil {
		return err
	}
	baseFi, err := os.Stat(base)
	if err != nil {
		return err
	}

	term := strings.ToLower(opts.Term)

	// A file root yields just that entry — used for single-file selections.
	if !baseFi.IsDir() {
		return s.walkFileRoot(baseRel, baseFi, opts, term, fn)
	}
	return s.walkDir(ctx, base, baseRel, term, opts, fn)
}

var defaultNoiseDirs = map[string]struct{}{
	".git":                      {},
	".svn":                      {},
	".hg":                       {},
	"node_modules":              {},
	"@eaDir":                    {}, // Synology NAS thumbnails
	"#recycle":                  {}, // Synology NAS recycle bin
	"@Recycle":                  {}, // QNAP NAS recycle bin
	".@__thumb":                 {}, // QNAP NAS thumbnails
	"$RECYCLE.BIN":              {}, // Windows recycle bin
	"System Volume Information": {},
	".Trash-1000":               {}, // Linux trash
	".Trashes":                  {}, // macOS trash
	".fseventsd":                {}, // macOS filesystem events
	".Spotlight-V100":           {}, // macOS spotlight
	".cache":                    {},
	"__pycache__":               {},
	".venv":                     {},
}

func shouldSkipDir(name, lowerTerm string, isSearch bool) bool {
	if name == reservedDir {
		return true
	}
	if !isSearch {
		return false
	}
	if lowerTerm != "" {
		lowerName := strings.ToLower(name)
		if strings.Contains(lowerTerm, lowerName) || strings.Contains(lowerName, lowerTerm) {
			return false
		}
	}
	_, skip := defaultNoiseDirs[name]
	return skip
}

type dirTask struct {
	diskPath string
	relPath  string
}

type walkContext struct {
	opts         storage.SearchOptions
	term         string
	isSearch     bool
	callFn       func(storage.FileInfo) error
	recordErr    func(error)
	limitReached *atomic.Bool
}

func (s *Storage) matchDir(opts storage.SearchOptions, lowerTerm, fullRel, name string) bool {
	if opts.Type != "" && opts.Type != storage.TypeDir {
		return false
	}
	if opts.Extension != "" {
		return false
	}
	return matchTerm(lowerTerm, fullRel, name)
}

func (s *Storage) matchFile(opts storage.SearchOptions, lowerTerm, fullRel, name string) bool {
	if opts.Type == storage.TypeDir {
		return false
	}
	if opts.Extension != "" {
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
		if ext != opts.Extension {
			return false
		}
	}
	if opts.Type != "" {
		detectedType, _ := fsutil.Detect(name, nil)
		if detectedType != opts.Type {
			return false
		}
	}
	return matchTerm(lowerTerm, fullRel, name)
}

func matchTerm(lowerTerm, fullRel, name string) bool {
	if lowerTerm == "" {
		return true
	}
	if strings.Contains(strings.ToLower(name), lowerTerm) {
		return true
	}
	return strings.Contains(strings.ToLower(fullRel), lowerTerm)
}

func (s *Storage) processDir(wc *walkContext, task dirTask, d fs.DirEntry, childRel string, nextQueue *[]dirTask) bool {
	name := d.Name()
	if shouldSkipDir(name, wc.term, wc.isSearch) {
		return true
	}

	if s.matchDir(wc.opts, wc.term, childRel, name) {
		fi, err := d.Info()
		if err == nil {
			item := s.info(childRel, fi, nil)
			if err := wc.callFn(item); err != nil {
				wc.recordErr(err)
				wc.limitReached.Store(true)
				return false
			}
		}
	}

	*nextQueue = append(*nextQueue, dirTask{
		diskPath: filepath.Join(task.diskPath, name),
		relPath:  childRel,
	})
	return true
}

func (s *Storage) processFile(wc *walkContext, d fs.DirEntry, childRel string) bool {
	name := d.Name()
	if !s.matchFile(wc.opts, wc.term, childRel, name) {
		return true
	}

	fi, err := d.Info()
	if err != nil {
		return true
	}

	item := s.info(childRel, fi, nil)
	if err := wc.callFn(item); err != nil {
		wc.recordErr(err)
		wc.limitReached.Store(true)
		return false
	}
	return true
}

func (s *Storage) scanTask(ctx context.Context, wc *walkContext, task dirTask, nextQueue *[]dirTask) {
	entries, err := os.ReadDir(task.diskPath)
	if err != nil {
		return
	}

	for _, d := range entries {
		if wc.limitReached.Load() || ctx.Err() != nil {
			return
		}

		childRel := d.Name()
		if task.relPath != "." && task.relPath != "" {
			childRel = task.relPath + "/" + d.Name()
		}

		if d.IsDir() {
			if !s.processDir(wc, task, d, childRel, nextQueue) {
				return
			}
		} else {
			if !s.processFile(wc, d, childRel) {
				return
			}
		}
	}
}

func (s *Storage) walkLevel(ctx context.Context, wc *walkContext, current []dirTask, numWorkers int) []dirTask {
	workerNext := make([][]dirTask, numWorkers)
	var taskIdx atomic.Int64
	var wg sync.WaitGroup

	actualWorkers := min(numWorkers, len(current))
	wg.Add(actualWorkers)

	for w := range actualWorkers {
		workerID := w
		go func() {
			defer wg.Done()
			for {
				if wc.limitReached.Load() || ctx.Err() != nil {
					return
				}
				i := int(taskIdx.Add(1) - 1)
				if i >= len(current) {
					return
				}
				s.scanTask(ctx, wc, current[i], &workerNext[workerID])
			}
		}()
	}

	wg.Wait()

	var next []dirTask
	for _, sub := range workerNext {
		next = append(next, sub...)
	}
	return next
}

func newWalkContext(opts storage.SearchOptions, term string, fn func(storage.FileInfo) error) (*walkContext, func() error) {
	isSearch := opts.Term != "" || opts.Type != "" || opts.Extension != ""
	limit := opts.Limit

	var (
		fnMu         sync.Mutex
		limitReached atomic.Bool
		firstErr     error
		errOnce      sync.Once
	)

	wc := &walkContext{
		opts:         opts,
		term:         term,
		isSearch:     isSearch,
		limitReached: &limitReached,
		recordErr: func(err error) {
			if err != nil && !errors.Is(err, storage.ErrDone) && !errors.Is(err, context.Canceled) {
				errOnce.Do(func() { firstErr = err })
			}
		},
		callFn: func(item storage.FileInfo) error {
			fnMu.Lock()
			defer fnMu.Unlock()
			if limitReached.Load() {
				return storage.ErrDone
			}
			if err := fn(item); err != nil {
				return err
			}
			if limit > 0 {
				limit--
				if limit == 0 {
					limitReached.Store(true)
					return storage.ErrDone
				}
			}
			return nil
		},
	}
	return wc, func() error { return firstErr }
}

// walkDir visits every entry below the base directory using parallel breadth-first search.
// It filters entries using directory metadata before calling stat (d.Info()), prunes noise
// directories during search, and streams matches level-by-level with bounded concurrency.
func (s *Storage) walkDir(ctx context.Context, base, baseRel, term string, opts storage.SearchOptions, fn func(storage.FileInfo) error) error {
	wc, getErr := newWalkContext(opts, term, fn)
	currentQueue := []dirTask{{diskPath: base, relPath: baseRel}}
	numWorkers := min(16, max(4, runtime.GOMAXPROCS(0)*2))

	for len(currentQueue) > 0 {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if wc.limitReached.Load() {
			break
		}

		currentQueue = s.walkLevel(ctx, wc, currentQueue, numWorkers)
		if err := getErr(); err != nil {
			return err
		}
	}

	return getErr()
}

// walkFileRoot reports the single entry when the walk root is a file.
func (s *Storage) walkFileRoot(rel string, fi fs.FileInfo, opts storage.SearchOptions, term string, fn func(storage.FileInfo) error) error {
	item := s.info(rel, fi, nil)
	if !matches(opts, term, item) {
		return nil
	}
	if err := fn(item); err != nil && !errors.Is(err, storage.ErrDone) {
		return err
	}
	return nil
}

// matches applies the search filters to an entry.
func matches(opts storage.SearchOptions, lowerTerm string, item storage.FileInfo) bool {
	if lowerTerm != "" && !strings.Contains(strings.ToLower(item.Path), lowerTerm) {
		return false
	}
	if opts.Type != "" && opts.Type != item.Type {
		return false
	}
	if opts.Extension != "" && opts.Extension != item.Extension {
		return false
	}
	return true
}

// StartUpload begins a chunked upload at path, truncating any existing
// partial (the tus offset is defined as the on-disk size).
func (s *Storage) StartUpload(_ context.Context, p string, _ int64) (storage.Upload, error) {
	_, target, err := s.resolve(p)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	// O_TRUNC: the tus offset is defined as the on-disk size, so any
	// preexisting partial data is discarded and Offset starts at 0.
	// (No preallocation: it would inflate the reported offset.)
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &upload{f: f, path: target}, nil
}

// validateMove checks the invariants of a move: neither endpoint is the
// root, source differs from destination, and a directory is never moved
// below itself ("a" → "a/b/c").
func (s *Storage) validateMove(fromTarget, toTarget string) error {
	if fromTarget == s.evalRoot || toTarget == s.evalRoot {
		return os.ErrPermission
	}
	if fromTarget == toTarget {
		return os.ErrInvalid
	}
	if rel, err := filepath.Rel(fromTarget, toTarget); err == nil && rel != "." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return os.ErrInvalid
	}
	return nil
}

// Move renames from to to. The destination must not exist, and a directory
// is never moved into itself or one of its descendants.
func (s *Storage) Move(_ context.Context, from, to string) (storage.FileInfo, error) {
	_, fromTarget, err := s.resolve(from)
	if err != nil {
		return storage.FileInfo{}, err
	}
	toRel, toTarget, err := s.resolve(to)
	if err != nil {
		return storage.FileInfo{}, err
	}
	if err := s.validateMove(fromTarget, toTarget); err != nil {
		return storage.FileInfo{}, err
	}
	if _, err := os.Lstat(toTarget); err == nil {
		return storage.FileInfo{}, os.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return storage.FileInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(toTarget), 0o755); err != nil {
		return storage.FileInfo{}, err
	}
	if err := os.Rename(fromTarget, toTarget); err != nil {
		return storage.FileInfo{}, err
	}
	fi, err := os.Stat(toTarget)
	if err != nil {
		return storage.FileInfo{}, err
	}
	return s.info(toRel, fi, nil), nil
}

// Write replaces or creates the file at path atomically: data is staged in
// a temp file inside the destination directory and renamed into place, so
// readers never observe a partial file.
func (s *Storage) Write(_ context.Context, p string, r io.Reader, size int64) (storage.FileInfo, error) {
	rel, target, err := s.resolve(p)
	if err != nil {
		return storage.FileInfo{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return storage.FileInfo{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".fbwrite-*")
	if err != nil {
		return storage.FileInfo{}, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := io.Copy(tmp, io.LimitReader(r, size)); err != nil {
		cleanup()
		return storage.FileInfo{}, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return storage.FileInfo{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return storage.FileInfo{}, err
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return storage.FileInfo{}, err
	}
	fi, err := os.Stat(target)
	if err != nil {
		return storage.FileInfo{}, err
	}
	return s.info(rel, fi, nil), nil
}

// Remove deletes path (including non-empty directories). It backs internal
// maintenance such as abandoned-upload GC; delete endpoints must expose it
// only to authenticated administrators.
func (s *Storage) Remove(_ context.Context, p string) error {
	_, target, err := s.resolve(p)
	if err != nil {
		return err
	}
	if target == s.evalRoot {
		return os.ErrPermission // never remove the root itself
	}
	return os.RemoveAll(target)
}

// upload implements storage.Upload on a local file.
type upload struct {
	f    *os.File
	path string
}

func (u *upload) Offset() (int64, error) {
	fi, err := u.f.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

func (u *upload) WriteAt(p []byte, off int64) (int, error) {
	return u.f.WriteAt(p, off)
}

func (u *upload) Commit() error {
	if err := u.f.Sync(); err != nil {
		_ = u.f.Close()
		return err
	}
	return u.f.Close()
}

func (u *upload) Abort() error {
	_ = u.f.Close()
	return os.Remove(u.path)
}
