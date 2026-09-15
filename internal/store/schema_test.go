package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// planTables is the complete PLAN.md §5 table list, written here as literals:
// the point of TestSchema is that the table set is complete and the version
// string is explicit, so the expectation must not be derived from schema.go.
// (Phase 3 extends the schema only through the guarded migration in migrate.go's
// tests — this test still pins the tables and the bootstrap.)
var planTables = []string{
	"players", "items", "sealed_bids", "bids", "auction_runtime", "game", "events",
}

// nicknameKeyIndex is D-28's UNIQUE index name, pinned as a literal so a rename
// fails here rather than in a query nobody runs until the venue.
const nicknameKeyIndex = "idx_players_nickname_key"

// TestSchema pins D-36: one idempotent DDL pass, the current user_version, the
// complete PLAN.md §5 schema plus the players.nickname_key UNIQUE index, and the
// single bootstrapped game row.
func TestSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()

	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema: %v", err)
	}

	t.Run("creates every PLAN section 5 table", func(t *testing.T) {
		for _, name := range planTables {
			var got string
			err := st.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&got)
			if err != nil {
				t.Errorf("table %q is missing from the schema: %v", name, err)
			}
		}
	})

	t.Run("declares the players.nickname_key UNIQUE index", func(t *testing.T) {
		var sqlText string
		err := st.DB.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, nicknameKeyIndex).Scan(&sqlText)
		if err != nil {
			t.Fatalf("index %q is missing: %v", nicknameKeyIndex, err)
		}
		if !containsFold(sqlText, "unique") {
			t.Errorf("index %q is not UNIQUE: %s", nicknameKeyIndex, sqlText)
		}
	})

	t.Run("reports the current user_version", func(t *testing.T) {
		var got int
		if err := st.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
			t.Fatalf("PRAGMA user_version: %v", err)
		}
		if got != UserVersion {
			t.Errorf("user_version = %d, want %d", got, UserVersion)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		// A second boot against an existing file must change nothing — the demo
		// database is disposable, but every restart runs this pass.
		if err := Schema(st.DB); err != nil {
			t.Fatalf("a second Schema pass failed: %v", err)
		}
		var got int
		if err := st.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
			t.Fatalf("PRAGMA user_version: %v", err)
		}
		if got != UserVersion {
			t.Errorf("user_version = %d after a second pass, want %d", got, UserVersion)
		}
		for _, name := range planTables {
			var found string
			if err := st.DB.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found); err != nil {
				t.Errorf("table %q disappeared on the second pass: %v", name, err)
			}
		}
	})

	t.Run("bootstraps the single canonical game row", func(t *testing.T) {
		now := time.UnixMilli(1_700_000_000_000)
		if err := Bootstrap(st.DB, now); err != nil {
			t.Fatalf("Bootstrap: %v", err)
		}
		if err := Bootstrap(st.DB, now); err != nil {
			t.Fatalf("a second Bootstrap failed: %v", err)
		}

		var rows int
		if err := st.DB.QueryRow(`SELECT COUNT(*) FROM game`).Scan(&rows); err != nil {
			t.Fatalf("counting game rows: %v", err)
		}
		if rows != 1 {
			t.Errorf("the game table holds %d rows, want exactly 1", rows)
		}

		got, err := st.LoadGame()
		if err != nil {
			t.Fatalf("LoadGame: %v", err)
		}
		if got.Phase != "LOBBY" {
			t.Errorf("bootstrapped phase = %q, want %q", got.Phase, "LOBBY")
		}
		if got.Revision != 0 {
			t.Errorf("bootstrapped revision = %d, want 0", got.Revision)
		}
		if got.CurrentItemID != "" {
			t.Errorf("bootstrapped current_item_id = %q, want empty", got.CurrentItemID)
		}
		for _, want := range []string{`"starting_budget":1000000`, `"currency":"VND"`} {
			if !containsFold(got.ConfigJSON, want) {
				t.Errorf("bootstrapped config_json = %q, want it to contain %s", got.ConfigJSON, want)
			}
		}
	})

	t.Run("players.nickname_key rejects a case-insensitive duplicate", func(t *testing.T) {
		insert := `INSERT INTO players (id, nickname, nickname_key, token, budget, total_value, joined_at, last_seen_at)
		           VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
		if _, err := st.DB.Exec(insert, "p-1", "An", "an", "tok-1", 1_000_000, 0, 1, 1); err != nil {
			t.Fatalf("inserting the first player: %v", err)
		}
		// "AN" passes the case-SENSITIVE nickname constraint, so only the
		// nickname_key index can refuse it — which is exactly D-28's point:
		// SQLite's NOCASE would fold ASCII but not Vietnamese.
		_, err := st.DB.Exec(insert, "p-2", "AN", "an", "tok-2", 1_000_000, 0, 2, 2)
		if err == nil {
			t.Fatal("a case-insensitive duplicate nickname_key was accepted; uniqueness is not enforced by the schema")
		}
		if !IsUniqueViolation(err) {
			t.Errorf("the failure is not a SQLite UNIQUE violation (code 2067): %v", err)
		}
	})
}

// TestStorePoolIsSingleConnection is research Pitfall 10's fix: SQLite has one
// writer, and an unbounded database/sql pool turns a concurrent write into
// SQLITE_BUSY — which D-34 escalates into a process exit mid-auction.
func TestStorePoolIsSingleConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()

	if got := st.DB.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("MaxOpenConnections = %d, want 1: the pool must not allow a second writer", got)
	}
}

// containsFold is a case-insensitive substring check so the assertions above can
// name what they look for without caring how the driver spells it back.
func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
