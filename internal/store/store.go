// Package store owns the SQLite handle: the byte-exact pragma DSN and the
// fail-hard boot assertion that keeps the database in WAL mode.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the driver name "sqlite", never "sqlite3"
)

// Store holds the process-wide SQLite handle. Phase 1 creates no schema.
type Store struct {
	DB *sql.DB
}

// DSN builds the connection string. Every pragma lives here and only here:
// pragmas are per-connection, and `db.Exec("PRAGMA …")` after Open would
// configure whichever pooled connection happened to be checked out.
//
// The `_pragma=` form is deliberate — the shorthand `?_journal_mode(WAL)` with
// the wrong key fails *silently* (journal_mode stays "delete", no error), which
// is why Open re-reads the mode and refuses to hand back a handle unless it is
// "wal".
func DSN(path string) string {
	return "file:" + path +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"
}

// Open opens the database and returns an error — never a handle — unless
// SQLite reports journal_mode = wal.
//
// The journal_mode read is the first statement after sql.Open on purpose:
// database/sql.Open is lazy and happily "succeeds" on an unwritable path, so
// only a real statement surfaces the failure (boot fails hard instead of
// listening and then erroring under load).
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		return nil, fmt.Errorf("sqlite open %q: %w", path, err)
	}

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite journal_mode probe %q: %w", path, err)
	}
	if mode != "wal" {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite journal_mode is %q, want \"wal\" for %q: pragmas must come from store.DSN", mode, path)
	}

	return &Store{DB: db}, nil
}

// Close releases the handle.
func (s *Store) Close() error {
	return s.DB.Close()
}
