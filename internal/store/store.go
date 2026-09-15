// Package store owns the SQLite handle: the byte-exact pragma DSN and the
// fail-hard boot assertion that keeps the database in WAL mode.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the driver name "sqlite", never "sqlite3"
)

// PragmaState is the state Open observed on the live handle. The boot log and
// /api/health report these facts instead of re-stating what the DSN intends:
// the health `wal` field is the exact smoke check the D-19 contract points at,
// so it must come from the assertion, not from a compile-time constant.
type PragmaState struct {
	JournalMode string // always "wal" for a handle Open returned
	BusyTimeout int    // milliseconds
	ForeignKeys bool
}

// Store holds the process-wide SQLite handle. Phase 1 creates no schema.
type Store struct {
	DB      *sql.DB
	pragmas PragmaState
}

// Pragmas returns the state observed at Open time.
func (s *Store) Pragmas() PragmaState { return s.pragmas }

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

	// Read the two other pragmas back from a live connection too, so the boot
	// log reports observed values instead of parroting the DSN. Every pooled
	// connection gets its pragmas from the DSN, so whichever connection answers
	// here reports the same state the request path will see.
	var busyTimeout, foreignKeys int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite busy_timeout probe %q: %w", path, err)
	}
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite foreign_keys probe %q: %w", path, err)
	}

	return &Store{
		DB: db,
		pragmas: PragmaState{
			JournalMode: mode,
			BusyTimeout: busyTimeout,
			ForeignKeys: foreignKeys == 1,
		},
	}, nil
}

// Close releases the handle.
func (s *Store) Close() error {
	return s.DB.Close()
}
