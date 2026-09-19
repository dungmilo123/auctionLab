package store

import (
	"database/sql"
	"encoding/json"
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
		for _, want := range []string{`"starting_budget":1000000`, `"currency":"USD"`} {
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

// ---------------------------------------------------------------------------
// D-02 (quick task 260919-xbg): the user_version 3→4 currency migration. The
// fixture is written as a literal — the config_json the previous build seeded —
// so the step is proved against a file that build could actually have produced.
// ---------------------------------------------------------------------------

// v3GameConfigJSON is the old bootstrap literal, plus the Phase 4 join_url key:
// the migration must rewrite the currency and leave every other key alone.
const v3GameConfigJSON = `{"starting_budget":1000000,"currency":"VND","join_url":"http://192.168.0.103:8080/"}`

// v3GameFile writes a v3 file by hand: the full v4 schema (the two versions
// differ only in data, so the DDL is not re-listed) with a VND game row and
// user_version pushed back to 3 — exactly what a boot before this build left
// behind.
func v3GameFile(t *testing.T, path string) *Store {
	t.Helper()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if _, err := st.DB.Exec(
		`INSERT INTO game (id, phase, current_item_id, revision, config_json, updated_at)
		 VALUES (1, 'LOBBY', NULL, 0, ?, 1)`, v3GameConfigJSON); err != nil {
		t.Fatalf("writing the v3 game row: %v", err)
	}
	setUserVersion(t, st.DB, 3)
	return st
}

// storedConfigJSON reads game.config_json back verbatim.
func storedGameConfigJSON(t *testing.T, db *sql.DB) string {
	t.Helper()

	var raw string
	if err := db.QueryRow(`SELECT config_json FROM game WHERE id = 1`).Scan(&raw); err != nil {
		t.Fatalf("reading game.config_json: %v", err)
	}
	return raw
}

// TestMigrationV3ToV4Currency is D-02's contract: a v3 file's persisted currency
// becomes USD in the same guarded transaction as the version bump, the budget
// amount is retained rather than converted, and every other config key survives
// the re-encode. A second pass and a fresh bootstrap both leave the value alone.
func TestMigrationV3ToV4Currency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st := v3GameFile(t, path)

	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema on a v3 file: %v", err)
	}

	t.Run("reports the current user_version", func(t *testing.T) {
		var got int
		if err := st.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
			t.Fatalf("PRAGMA user_version: %v", err)
		}
		if got != UserVersion {
			t.Errorf("user_version = %d, want %d", got, UserVersion)
		}
	})

	t.Run("rewrites the currency to USD and keeps every other key", func(t *testing.T) {
		var cfg map[string]any
		if err := json.Unmarshal([]byte(storedGameConfigJSON(t, st.DB)), &cfg); err != nil {
			t.Fatalf("parsing the migrated config_json: %v", err)
		}
		if cfg["currency"] != currencyUSD {
			t.Errorf("currency = %v, want %q", cfg["currency"], currencyUSD)
		}
		// The amount is retained, not converted: the game's integers are its own
		// accounting units (D-02).
		if budget, ok := cfg["starting_budget"].(float64); !ok || budget != 1_000_000 {
			t.Errorf("starting_budget = %v, want the retained 1000000", cfg["starting_budget"])
		}
		if cfg["join_url"] != "http://192.168.0.103:8080/" {
			t.Errorf("join_url = %v, want the untouched selection — the re-encode is not a rewrite", cfg["join_url"])
		}
	})

	t.Run("a second pass does not re-issue the step", func(t *testing.T) {
		after := storedGameConfigJSON(t, st.DB)
		if err := Schema(st.DB); err != nil {
			t.Fatalf("a second Schema pass failed: %v", err)
		}
		if got := storedGameConfigJSON(t, st.DB); got != after {
			t.Errorf("config_json = %q after a second pass, want the unchanged %q", got, after)
		}
	})
}

// TestMigrationV4LeavesAUSDOrAKeylessFileAlone pins the guard's two other arms:
// a file that already says USD is not rewritten (so a presenter's edit survives
// a boot), and a file with no game row yet — a fresh database, where Bootstrap
// runs after the migration — is left for the bootstrap to seed.
func TestMigrationV4LeavesAUSDOrAKeylessFileAlone(t *testing.T) {
	t.Run("an already-USD file is byte-for-byte untouched", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auction.db")
		st := openMigrateStore(t, path)
		if err := Schema(st.DB); err != nil {
			t.Fatalf("Schema on a fresh file: %v", err)
		}
		const current = `{"starting_budget":2000000,"currency":"USD"}`
		if _, err := st.DB.Exec(
			`INSERT INTO game (id, phase, current_item_id, revision, config_json, updated_at)
			 VALUES (1, 'LOBBY', NULL, 0, ?, 1)`, current); err != nil {
			t.Fatalf("writing the game row: %v", err)
		}
		setUserVersion(t, st.DB, 3)

		if err := Schema(st.DB); err != nil {
			t.Fatalf("Schema: %v", err)
		}
		if got := storedGameConfigJSON(t, st.DB); got != current {
			t.Errorf("config_json = %q, want the untouched %q", got, current)
		}
	})

	t.Run("a fresh file has no row and is left to Bootstrap", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "auction.db")
		st := openMigrateStore(t, path)
		if err := Schema(st.DB); err != nil {
			t.Fatalf("Schema: %v", err)
		}

		var n int
		if err := st.DB.QueryRow(`SELECT COUNT(*) FROM game`).Scan(&n); err != nil {
			t.Fatalf("counting game rows: %v", err)
		}
		if n != 0 {
			t.Fatalf("the fresh file carries %d game rows before Bootstrap, want 0", n)
		}

		now := time.UnixMilli(1_700_000_000_000)
		if err := Bootstrap(st.DB, now); err != nil {
			t.Fatalf("Bootstrap: %v", err)
		}
		if got := storedGameConfigJSON(t, st.DB); !containsFold(got, `"currency":"USD"`) {
			t.Errorf("bootstrapped config_json = %q, want the USD default", got)
		}
	})
}
