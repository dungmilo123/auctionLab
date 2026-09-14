package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestDSNMatrix is the regression lock for the silent-pragma trap. It replays
// the four DSN variants measured on modernc.org/sqlite v1.58.0 and requires
// exactly `delete` / `delete` / `wal` / `wal` in table order.
//
// Every case uses its own fresh database file under t.TempDir(). That is not
// hygiene, it is the measurement: `journal_mode=wal` is persisted in the SQLite
// file header, so a reused path reports "wal" no matter which DSN opened it —
// the assertion would pass for the wrong reason (TestWALIsStickyInTheFileHeader
// documents exactly this).
func TestDSNMatrix(t *testing.T) {
	const (
		malformedTail = "?_journal_mode(WAL)&_txlock=immediate" // DSN key AND value shape are wrong
		shorthandTail = "?_journal_mode=WAL&_busy_timeout=5000" // mattn-style shorthand, accepted
	)

	cases := []struct {
		name string
		dsn  func(path string) string
		want string
	}{
		{
			name: "no DSN at all — driver defaults",
			dsn:  func(path string) string { return path },
			want: "delete",
		},
		{
			// The trap: `_journal_mode(WAL)` is not a pragma key modernc knows,
			// and the failure is silent — no error, journal_mode stays "delete".
			// A skeleton that "simplified" the DSN to this form would run the whole
			// presentation without WAL and nothing would say so.
			name: "malformed pragma is silently ignored",
			dsn:  func(path string) string { return "file:" + path + malformedTail },
			want: "delete",
		},
		{
			name: "the locked _pragma= form",
			dsn:  DSN,
			want: "wal",
		},
		{
			name: "mattn-style shorthand still works",
			dsn:  func(path string) string { return "file:" + path + shorthandTail },
			want: "wal",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auction.db")

			db, err := sql.Open("sqlite", tc.dsn(path))
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			defer func() { _ = db.Close() }()

			var mode string
			if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
				t.Fatalf("PRAGMA journal_mode: %v", err)
			}
			t.Logf("journal_mode = %q (want %q)", mode, tc.want)
			if mode != tc.want {
				t.Errorf("journal_mode = %q, want %q (dsn tail %q)",
					mode, tc.want, tc.dsn("/path/auction.db"))
			}
		})
	}
}

// TestWALIsStickyInTheFileHeader is a documentation test with a job: it proves
// why every matrix case and criterion 4 must use a fresh path.
//
// Unlike most journal modes, WAL is written into the SQLite file header, so it
// survives the connection that set it. Reopening the same file with no pragmas
// at all still reports "wal" — which means a `journal_mode` assertion run
// against an existing database measures the file's history, not the DSN under
// test.
func TestWALIsStickyInTheFileHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")

	// 1. One open with the locked DSN puts the file into WAL.
	first, err := Open(path)
	if err != nil {
		t.Fatalf("first Open(%q): %v", path, err)
	}
	var firstMode string
	if err := first.DB.QueryRow("PRAGMA journal_mode").Scan(&firstMode); err != nil {
		t.Fatalf("PRAGMA journal_mode (first open): %v", err)
	}
	if firstMode != "wal" {
		t.Fatalf("first open journal_mode = %q, want \"wal\" for the DSN to be worth testing", firstMode)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("closing the first handle: %v", err)
	}

	// 2. Reopen the same file with no pragma at all: still WAL.
	second, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("reopening %q: %v", path, err)
	}
	defer func() { _ = second.Close() }()

	var secondMode string
	if err := second.QueryRow("PRAGMA journal_mode").Scan(&secondMode); err != nil {
		t.Fatalf("PRAGMA journal_mode (second open): %v", err)
	}
	if secondMode != "wal" {
		t.Fatalf("reopening a WAL file without pragmas reported %q, want \"wal\". "+
			"If this ever fails the matrix above is measuring file history, not the DSN — "+
			"and criterion 4's fresh-path requirement deserves a re-read.", secondMode)
	}
}
