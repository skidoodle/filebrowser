package authz

import (
	"errors"
	"strings"
	"sync"

	"github.com/skidoodle/filebrowser/internal/fsutil"
)

// Actor is the permission-relevant slice of an identity. The zero value is
// an anonymous guest.
type Actor struct {
	UserID int64
	Admin  bool
	Scope  string // slash-relative prefix; empty = whole root
}

// Errors surfaced to the API layer for status mapping: private violations
// must look like 404 (hidden, not protected), out-of-scope writes are 403.
var (
	ErrPrivate    = errors.New("authz: path is private to someone else")
	ErrOutOfScope = errors.New("authz: path outside actor scope")
	ErrDenied     = errors.New("authz: denied")
)

// PrivateSource persists the private-folder set.
type PrivateSource interface {
	PrivateFolders() (map[string]int64, error)
	SetPrivate(dir string, ownerID int64) error
	UnsetPrivate(dir string) error
	RenamePrivate(from, to string) error
	DeletePrivateBelow(dir string) error
}

// Engine caches the private-folder set and evaluates permissions.
type Engine struct {
	src   PrivateSource
	mu    sync.RWMutex
	priv  map[string]int64 // private dir path → owner user id
	ready bool
}

// New builds an engine and loads the private set from src.
func New(src PrivateSource) (*Engine, error) {
	e := &Engine{src: src}
	if err := e.reload(); err != nil {
		return nil, err
	}
	return e, nil
}

// reload refreshes the cache. Callers hold no locks.
func (e *Engine) reload() error {
	priv, err := e.src.PrivateFolders()
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.priv = priv
	e.ready = true
	e.mu.Unlock()
	return nil
}

// Invalidate reloads the private set after out-of-band mutations such as
// user deletion (which cascades their private-folder rows away).
func (e *Engine) Invalidate() error { return e.reload() }

// clean normalizes a permission-check path; callers pass request paths.
func clean(p string) (string, error) {
	c, err := fsutil.Clean(p)
	if err != nil {
		return "", err
	}
	return c, nil
}

// InScope reports whether the cleaned path lies at or below the scope
// prefix (empty scope = whole root). Exported for the API's privacy toggle.
func InScope(scope, path string) bool {
	rel, err := clean(path)
	return err == nil && withinScope(scope, rel)
}

// withinScope reports whether path lies at or below the scope prefix
// (empty scope = whole root).
func withinScope(scope, rel string) bool {
	if scope == "" {
		return true
	}
	return rel == scope || strings.HasPrefix(rel, scope+"/")
}

// scopeOwnsRoot marks writes that target the scope folder itself rather
// than something inside it: a scoped user may never modify, rename,
// delete or replace their scope folder — only its contents.
func scopeOwnsRoot(scope, rel string) bool {
	return scope != "" && rel == scope
}

// owningPrivateFolder returns the most specific private folder containing
// rel, if any. Private folders may nest; the deepest one wins.
func (e *Engine) owningPrivateFolder(rel string) (string, int64, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	best, bestOwner, found := "", int64(0), false
	for p, owner := range e.priv {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			if len(p) > len(best) {
				best, bestOwner, found = p, owner, true
			}
		}
	}
	return best, bestOwner, found
}

// privateToOthers reports whether any private folder containing rel is
// owned by someone other than actor. Admins are exempt (they see all).
func (e *Engine) privateToOthers(actor Actor, rel string) bool {
	if actor.Admin {
		return false
	}
	_, owner, found := e.owningPrivateFolder(rel)
	return found && owner != actor.UserID
}

// CanRead reports whether actor may see path. Everything is public unless
// it falls inside a private folder owned by someone else; that violation
// is surfaced as ErrPrivate so the API can answer 404.
func (e *Engine) CanRead(actor Actor, path string) error {
	rel, err := clean(path)
	if err != nil {
		return err
	}
	if e.privateToOthers(actor, rel) {
		return ErrPrivate
	}
	return nil
}

// CanWrite reports whether actor may modify path. Admins write anywhere;
// scoped users write only inside their scope and never inside someone
// else's private folder; guests can never write.
func (e *Engine) CanWrite(actor Actor, path string) error {
	rel, err := clean(path)
	if err != nil {
		return err
	}
	switch {
	case actor.Admin:
		return nil
	case actor.UserID == 0:
		return ErrDenied
	}
	if !withinScope(actor.Scope, rel) || scopeOwnsRoot(actor.Scope, rel) {
		return ErrOutOfScope
	}
	if e.privateToOthers(actor, rel) {
		return ErrPrivate
	}
	return nil
}

// FilterPaths removes entries the actor must not see (private folders of
// others, including anything nested below them).
func (e *Engine) FilterPaths(actor Actor, paths []string) []string {
	out := paths[:0]
	for _, p := range paths {
		rel, err := clean(p)
		if err != nil {
			continue
		}
		if e.privateToOthers(actor, rel) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// PrivateOwner reports the owner of the private folder at exactly path.
func (e *Engine) PrivateOwner(path string) (int64, bool) {
	rel, err := clean(path)
	if err != nil {
		return 0, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	owner, ok := e.priv[rel]
	return owner, ok
}

// SetPrivate marks dir private for actor, enforcing that scoped users can
// only privatize directories inside their own scope and admins anywhere.
// Only directories may be marked; ownership of the path itself is checked
// by the caller (write permission).
func (e *Engine) SetPrivate(actor Actor, dir string) error {
	rel, err := clean(dir)
	if err != nil {
		return err
	}
	if rel == "." {
		return ErrDenied // the root itself cannot be private
	}
	if !actor.Admin {
		if actor.UserID == 0 || !withinScope(actor.Scope, rel) || scopeOwnsRoot(actor.Scope, rel) {
			return ErrOutOfScope
		}
	}
	if err := e.src.SetPrivate(rel, actor.UserID); err != nil {
		return err
	}
	return e.reload()
}

// UnsetPrivate clears the private mark on dir; only its owner or an admin
// may do so.
func (e *Engine) UnsetPrivate(actor Actor, dir string) error {
	rel, err := clean(dir)
	if err != nil {
		return err
	}
	if !actor.Admin {
		_, owner, found := e.owningPrivateFolder(rel)
		if !found || owner != actor.UserID {
			return ErrPrivate
		}
	}
	if err := e.src.UnsetPrivate(rel); err != nil {
		return err
	}
	return e.reload()
}

// RenamePrivate rewrites stored private paths after a move/rename so a
// relocated private folder stays private. Any authenticated writer may
// trigger this; the write permission on the move itself was already
// checked by the caller.
func (e *Engine) RenamePrivate(from, to string) error {
	if err := e.src.RenamePrivate(from, to); err != nil {
		return err
	}
	return e.reload()
}

// DeletePrivateBelow drops private marks at or below a deleted path.
func (e *Engine) DeletePrivateBelow(dir string) error {
	if err := e.src.DeletePrivateBelow(dir); err != nil {
		return err
	}
	return e.reload()
}
