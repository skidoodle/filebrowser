package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver" // register sqlite3 driver

	"github.com/skidoodle/filebrowser/internal/fsutil"
)

// User is one account row.
type User struct {
	ID           int64
	Username     string
	PasswordHash []byte
	Admin        bool
	Scope        string // slash-relative prefix; empty = whole root
	IsOriginal   bool   // the imported/seeded admin; cannot be deleted
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Sentinel errors mapped to HTTP statuses by the API layer.
var (
	ErrExists          = errors.New("store: username already taken")
	ErrNotFound        = errors.New("store: not found")
	ErrOriginal        = errors.New("store: original account is protected")
	ErrLastAdmin       = errors.New("store: cannot remove the last admin")
	ErrInvalid         = errors.New("store: invalid value")
	schemaVersion      = "1"
	errUniqueViolation = errors.New("UNIQUE constraint failed")
)

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and applies
// pragmas and migrations.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("store: create db dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.ExecContext(context.Background(), pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("store: %s: %w", pragma, err)
		}
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// migrate creates the schema; a future schema change bumps schemaVersion
// and adds a migration step keyed on meta.schema_version.
func (s *Store) migrate() error {
	const schema = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	is_admin INT NOT NULL DEFAULT 0,
	scope TEXT NOT NULL DEFAULT '',
	is_original INT NOT NULL DEFAULT 0,
	created_at INT NOT NULL,
	updated_at INT NOT NULL
);
CREATE TABLE IF NOT EXISTS private_folders (
	path TEXT PRIMARY KEY,
	owner_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT
);`
	if _, err := s.db.ExecContext(context.Background(), schema); err != nil {
		return fmt.Errorf("store: schema: %w", err)
	}
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO meta(key, value) VALUES('schema_version', ?)
		 ON CONFLICT(key) DO NOTHING`, schemaVersion); err != nil {
		return fmt.Errorf("store: schema_version: %w", err)
	}
	return nil
}

// ValidUsername reports whether name is acceptable: 1-64 chars from a safe
// printable set so usernames can never smuggle path or control characters.
func ValidUsername(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// ValidScope validates a scope prefix with fsutil.Clean semantics; the
// directory does not have to exist (a scope is a prefix, not a stat).
func ValidScope(scope string) (string, error) {
	clean, err := fsutil.Clean(scope)
	if err != nil {
		return "", ErrInvalid
	}
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

// CleanPath validates a private-folder path the same way.
func CleanPath(p string) (string, error) {
	clean, err := fsutil.Clean(p)
	if err != nil || clean == "." {
		return "", ErrInvalid
	}
	return clean, nil
}

// CreateUser inserts a new account.
func (s *Store) CreateUser(username string, passwordHash []byte, admin bool, scope string) (User, error) {
	if !ValidUsername(username) {
		return User{}, ErrInvalid
	}
	cleanScope, err := ValidScope(scope)
	if err != nil {
		return User{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(context.Background(),
		`INSERT INTO users(username, password_hash, is_admin, scope, is_original, created_at, updated_at)
		 VALUES(?, ?, ?, ?, 0, ?, ?)`,
		username, string(passwordHash), boolInt(admin), cleanScope, now, now)
	if err != nil {
		if strings.Contains(err.Error(), errUniqueViolation.Error()) {
			return User{}, ErrExists
		}
		return User{}, fmt.Errorf("store: create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return User{}, fmt.Errorf("store: create user: %w", err)
	}
	return s.User(id)
}

// User loads a single account by id.
func (s *Store) User(id int64) (User, error) {
	return s.scanUser(s.db.QueryRowContext(context.Background(),
		`SELECT id, username, password_hash, is_admin, scope, is_original, created_at, updated_at
		 FROM users WHERE id = ?`, id))
}

// UserByName loads a single account by username.
func (s *Store) UserByName(username string) (User, error) {
	return s.scanUser(s.db.QueryRowContext(context.Background(),
		`SELECT id, username, password_hash, is_admin, scope, is_original, created_at, updated_at
		 FROM users WHERE username = ?`, username))
}

// ListUsers returns every account ordered by id.
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, username, password_hash, is_admin, scope, is_original, created_at, updated_at
		 FROM users ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []User
	for rows.Next() {
		u, err := scanUserRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// CountUsers returns the number of accounts.
func (s *Store) CountUsers() (int, error) {
	var n int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count users: %w", err)
	}
	return n, nil
}

// CountAdmins returns the number of admin accounts.
func (s *Store) CountAdmins() (int, error) {
	var n int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count admins: %w", err)
	}
	return n, nil
}

// SetPassword updates an account's password hash.
func (s *Store) SetPassword(id int64, passwordHash []byte) error {
	res, err := s.db.ExecContext(context.Background(), `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		string(passwordHash), time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("store: set password: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateUser changes the admin flag and/or scope with guardrails: the
// last admin can never be demoted.
func (s *Store) UpdateUser(id int64, admin bool, scope string) (User, error) {
	cleanScope, err := ValidScope(scope)
	if err != nil {
		return User{}, err
	}
	current, err := s.User(id)
	if err != nil {
		return User{}, err
	}
	if current.Admin && !admin {
		n, err := s.CountAdmins()
		if err != nil {
			return User{}, err
		}
		if n <= 1 {
			return User{}, ErrLastAdmin
		}
	}
	if _, err := s.db.ExecContext(context.Background(), `UPDATE users SET is_admin = ?, scope = ?, updated_at = ? WHERE id = ?`,
		boolInt(admin), cleanScope, time.Now().Unix(), id); err != nil {
		return User{}, fmt.Errorf("store: update user: %w", err)
	}
	return s.User(id)
}

// RenameUser changes an account's username. Uniqueness is enforced; all
// other attributes (sessions, private folders, scope) are untouched since
// they key on the user id.
func (s *Store) RenameUser(id int64, username string) (User, error) {
	if !ValidUsername(username) {
		return User{}, ErrInvalid
	}
	current, err := s.User(id)
	if err != nil {
		return User{}, err
	}
	if current.Username == username {
		return current, nil
	}
	_, err = s.db.ExecContext(context.Background(),
		`UPDATE users SET username = ?, updated_at = ? WHERE id = ?`,
		username, time.Now().Unix(), id)
	if err != nil {
		if strings.Contains(err.Error(), errUniqueViolation.Error()) {
			return User{}, ErrExists
		}
		return User{}, fmt.Errorf("store: rename user: %w", err)
	}
	return s.User(id)
}

// DeleteUser removes an account, except the original admin. Deleting the
// last admin is refused; the original admin can be deleted only when
// another admin exists.
func (s *Store) DeleteUser(id int64) error {
	current, err := s.User(id)
	if err != nil {
		return err
	}
	if current.IsOriginal {
		return ErrOriginal
	}
	if current.Admin {
		n, err := s.CountAdmins()
		if err != nil {
			return err
		}
		if n <= 1 {
			return ErrLastAdmin
		}
	}
	if _, err := s.db.ExecContext(context.Background(), `DELETE FROM users WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete user: %w", err)
	}
	return nil
}

// CreateOriginal seeds the is_original admin account with a bcrypt hash,
// or resets its password when the account already exists. It returns the
// user id.
func (s *Store) CreateOriginal(passwordHash []byte) (int64, error) {
	u, err := s.UserByName("admin")
	switch {
	case err == nil:
		if !u.IsOriginal {
			return 0, fmt.Errorf("store: user %q exists but is not the original admin", u.Username)
		}
		if err := s.SetPassword(u.ID, passwordHash); err != nil {
			return 0, err
		}
		return u.ID, nil
	case errors.Is(err, ErrNotFound):
		now := time.Now().Unix()
		res, ierr := s.db.ExecContext(context.Background(),
			`INSERT INTO users(username, password_hash, is_admin, scope, is_original, created_at, updated_at)
			 VALUES('admin', ?, 1, '', 1, ?, ?)`, string(passwordHash), now, now)
		if ierr != nil {
			return 0, fmt.Errorf("store: create original: %w", ierr)
		}
		return res.LastInsertId()
	default:
		return 0, err
	}
}

// SetPrivate marks dir as private for ownerID.
func (s *Store) SetPrivate(dir string, ownerID int64) error {
	clean, err := CleanPath(dir)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(context.Background(),
		`INSERT INTO private_folders(path, owner_id) VALUES(?, ?)
		 ON CONFLICT(path) DO UPDATE SET owner_id = excluded.owner_id`, clean, ownerID); err != nil {
		return fmt.Errorf("store: set private: %w", err)
	}
	return nil
}

// UnsetPrivate clears the private mark on dir. Unmarking a folder that is
// not private is a no-op.
func (s *Store) UnsetPrivate(dir string) error {
	clean, err := CleanPath(dir)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(context.Background(), `DELETE FROM private_folders WHERE path = ?`, clean); err != nil {
		return fmt.Errorf("store: unset private: %w", err)
	}
	return nil
}

// RenamePrivate rewrites stored private paths after a move/rename: paths
// equal to from become to; paths below from are re-rooted under to.
func (s *Store) RenamePrivate(from, to string) error {
	cleanFrom, err := CleanPath(from)
	if err != nil {
		return err
	}
	cleanTo, err := CleanPath(to)
	if err != nil {
		return err
	}
	rows, err := s.db.QueryContext(context.Background(), `SELECT path FROM private_folders`)
	if err != nil {
		return fmt.Errorf("store: rename private: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var affected []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return err
		}
		if p == cleanFrom || strings.HasPrefix(p, cleanFrom+"/") {
			affected = append(affected, p)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range affected {
		newPath := cleanTo + strings.TrimPrefix(p, cleanFrom)
		if _, err := s.db.ExecContext(context.Background(), `UPDATE private_folders SET path = ? WHERE path = ?`, newPath, p); err != nil {
			return fmt.Errorf("store: rename private: %w", err)
		}
	}
	return nil
}

// DeletePrivateBelow removes private marks at or below path (used when a
// directory containing private folders is deleted).
func (s *Store) DeletePrivateBelow(dir string) error {
	if dir == "." || dir == "" {
		if _, err := s.db.ExecContext(context.Background(), `DELETE FROM private_folders`); err != nil {
			return fmt.Errorf("store: delete private below: %w", err)
		}
		return nil
	}
	clean, err := CleanPath(dir)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(context.Background(), `DELETE FROM private_folders WHERE path = ? OR path LIKE ? ESCAPE '\'`,
		clean, escapeLike(clean)+"/%"); err != nil {
		return fmt.Errorf("store: delete private below: %w", err)
	}
	return nil
}

// escapeLike escapes LIKE wildcards in p.
func escapeLike(p string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(p)
}

// PrivateFolders returns the full private map (path → owner id). authz
// caches this and invalidates on every mutation.
func (s *Store) PrivateFolders() (map[string]int64, error) {
	rows, err := s.db.QueryContext(context.Background(), `SELECT path, owner_id FROM private_folders`)
	if err != nil {
		return nil, fmt.Errorf("store: private folders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]int64)
	for rows.Next() {
		var p string
		var owner int64
		if err := rows.Scan(&p, &owner); err != nil {
			return nil, err
		}
		out[p] = owner
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Store) scanUser(row *sql.Row) (User, error) {
	u, err := scanUserRows(row)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

// userScanner is satisfied by *sql.Row and *sql.Rows.
type userScanner interface{ Scan(dest ...any) error }

func scanUserRows(row userScanner) (User, error) {
	var u User
	var admin, original int
	var created, updated int64
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &admin, &u.Scope, &original, &created, &updated)
	if err != nil {
		return User{}, err
	}
	u.Admin = admin == 1
	u.IsOriginal = original == 1
	u.CreatedAt = time.Unix(created, 0)
	u.UpdatedAt = time.Unix(updated, 0)
	return u, nil
}
