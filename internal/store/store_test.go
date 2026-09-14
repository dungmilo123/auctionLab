package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// lockedDSN is the byte-exact contract for DSN("x").
//
// It is written out here as a literal on purpose: the point of the test is that
// a later "simplification" of the `_pragma=` form — e.g. to the shorthand
// `?_journal_mode(WAL)` that modernc ignores *silently* — cannot pass review.
// Building the expectation from the implementation would make the assertion
// tautological and defeat the entire trap.
const lockedDSN = "file:x" +
	"?_pragma=busy_timeout(5000)" +
	"&_pragma=journal_mode(WAL)" +
	"&_pragma=synchronous(NORMAL)" +
	"&_pragma=foreign_keys(1)"

// TestDSNIsExact pins the DSN string byte-for-byte.
func TestDSNIsExact(t *testing.T) {
	if got := DSN("x"); got != lockedDSN {
		t.Fatalf("DSN(\"x\") is not byte-exact.\n got: %q\nwant: %q\n"+
			"The pragmas must keep the `_pragma=` form: pragmas are per-connection, and the "+
			"shorthand `?_journal_mode(WAL)` form fails silently (journal_mode stays \"delete\").",
			got, lockedDSN)
	}
}

// TestOpen proves the handle Open hands back is really configured the way the
// DSN promises: WAL plus the two per-connection pragmas the project relies on.
//
// Every case opens a fresh database file in its own t.TempDir(): journal_mode is
// sticky in the SQLite file header (see TestWALIsStickyInTheFileHeader), so a
// reused file would report "wal" even with a broken DSN.
func TestOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()

	var mode string
	if err := st.DB.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want %q", mode, "wal")
	}

	var busyTimeout int
	if err := st.DB.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}

	var foreignKeys int
	if err := st.DB.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
}

// TestOpenUnwritablePath pins the fail-hard boot path: sql.Open is lazy, so the
// first real statement is what surfaces an unusable path. A handle returned here
// would let the server log "listening" and then fail on the first query.
func TestOpenUnwritablePath(t *testing.T) {
	// A nested directory that does not exist: SQLite cannot create the file.
	path := filepath.Join(t.TempDir(), "missing-dir", "auction.db")

	st, err := Open(path)
	if err == nil {
		_ = st.Close()
		t.Fatalf("Open(%q) returned a handle for an unwritable path; want an error", path)
	}
	if st != nil {
		t.Errorf("Open(%q) returned a non-nil handle alongside the error: %+v", path, st)
	}
}

// TestDefaultPathFromEnv pins the DB_PATH override (D-15).
func TestDefaultPathFromEnv(t *testing.T) {
	t.Run("DB_PATH wins when set", func(t *testing.T) {
		want := filepath.Join(t.TempDir(), "override.db")
		t.Setenv("DB_PATH", want)

		if got := PathFromEnv(); got != want {
			t.Errorf("PathFromEnv() = %q, want %q", got, want)
		}
	})

	t.Run("unset falls back to ./auction.db", func(t *testing.T) {
		t.Setenv("DB_PATH", "")

		if got := PathFromEnv(); got != "./auction.db" {
			t.Errorf("PathFromEnv() = %q, want %q", got, "./auction.db")
		}
	})
}

// TestOpenKeepsDriverNameHonest guards the one-character trap: modernc's driver
// registers as "sqlite"; "sqlite3" belongs to mattn and would fail at first use.
func TestOpenKeepsDriverNameHonest(t *testing.T) {
	found := false
	for _, name := range sql.Drivers() {
		if name == "sqlite" {
			found = true
		}
		if name == "sqlite3" {
			t.Errorf("driver %q is registered; modernc must register as \"sqlite\" only", name)
		}
	}
	if !found {
		t.Error("driver \"sqlite\" is not registered; modernc.org/sqlite import is missing")
	}
}
