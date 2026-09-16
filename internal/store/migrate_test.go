package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// This file locks D-50: the v1 → v2 migration is a guarded, idempotent
// ALTER TABLE that lives in the same transaction as the version bump, and the
// schema pass can never lower a version it did not write.
//
// The pre-Phase-3 shape is written here as a literal (never derived from
// schema.go), so the test proves the migration against a file the old binary
// could actually have produced — exactly what research §1–2 executed.

// v1AuctionRuntimeDDL is PLAN.md §5's `auction_runtime` as shipped at
// user_version 1: no `extensions_used`, so the migration has real work to do.
const v1AuctionRuntimeDDL = `
CREATE TABLE auction_runtime (
  item_id     TEXT PRIMARY KEY,
  started_at  INTEGER NOT NULL,
  deadline_at INTEGER,
  settled_at  INTEGER,
  revision    INTEGER NOT NULL
);`

// openMigrateStore opens a bare handle (schema NOT applied) so the test can
// write a v1 file by hand first.
func openMigrateStore(t *testing.T, path string) *Store {
	t.Helper()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// setUserVersion writes the pragma directly; PRAGMA cannot be parameterized and
// every value here is a test literal.
func setUserVersion(t *testing.T, db *sql.DB, version int) {
	t.Helper()

	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		t.Fatalf("setting user_version = %d: %v", version, err)
	}
}

// hasColumn reports whether table carries the named column.
func hasColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()

	// PRAGMA table_info cannot be parameterized; the names are test literals.
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	found := false
	for rows.Next() {
		var (
			cid, notNull, pk int
			name, typ        string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scanning table_info(%s): %v", table, err)
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading table_info(%s): %v", table, err)
	}
	return found
}

// TestMigrationV1ToV2 is D-50's contract: a populated v1 file migrates once to
// the current UserVersion (the v1→v2 ALTER plus every later additive step), the
// new column is backfilled for the row that existed before the migration, and a
// second boot does not re-issue the ALTER (a blind second ALTER fails with
// "duplicate column name", which is NOT a UNIQUE violation and would therefore
// hit D-34's fatal path — research §1).
func TestMigrationV1ToV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st := openMigrateStore(t, path)

	// A v1 file with one populated runtime row: the migration must preserve it.
	if _, err := st.DB.Exec(v1AuctionRuntimeDDL); err != nil {
		t.Fatalf("writing the v1 auction_runtime DDL: %v", err)
	}
	if _, err := st.DB.Exec(
		`INSERT INTO auction_runtime (item_id, started_at, deadline_at, settled_at, revision)
		 VALUES ('it-legacy', 111, 222, NULL, 3)`); err != nil {
		t.Fatalf("inserting the pre-migration runtime row: %v", err)
	}
	setUserVersion(t, st.DB, 1)

	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema on a v1 file: %v", err)
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

	t.Run("adds extensions_used and backfills the existing row with 0", func(t *testing.T) {
		if !hasColumn(t, st.DB, "auction_runtime", "extensions_used") {
			t.Fatal("auction_runtime does not have extensions_used after the migration")
		}
		var (
			startedAt  int64
			extensions int
		)
		err := st.DB.QueryRow(
			`SELECT started_at, extensions_used FROM auction_runtime WHERE item_id = 'it-legacy'`).
			Scan(&startedAt, &extensions)
		if err != nil {
			t.Fatalf("reading the migrated row: %v", err)
		}
		if startedAt != 111 {
			t.Errorf("started_at = %d after the migration, want the pre-migration value 111", startedAt)
		}
		if extensions != 0 {
			t.Errorf("extensions_used = %d for the pre-migration row, want the column default 0", extensions)
		}
	})

	t.Run("a second schema pass does not re-issue the ALTER", func(t *testing.T) {
		if err := Schema(st.DB); err != nil {
			t.Fatalf("a second Schema pass on a v2 file failed: %v — the version guard must skip the ALTER", err)
		}
		var got int
		if err := st.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
			t.Fatalf("PRAGMA user_version: %v", err)
		}
		if got != UserVersion {
			t.Errorf("user_version = %d after a second pass, want %d", got, UserVersion)
		}
	})
}

// TestMigrationSurvivesTheCrashWindow is research §2's failure mode: a boot that
// ALTERed and died before the version bump leaves a file where the column
// exists but user_version is still 1. A guard that trusted the version alone
// would re-run the ALTER, fail with "duplicate column name", and hit the D-34
// fatal path forever — the column probe is what makes that boot recover.
func TestMigrationSurvivesTheCrashWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st := openMigrateStore(t, path)

	if _, err := st.DB.Exec(v1AuctionRuntimeDDL); err != nil {
		t.Fatalf("writing the v1 auction_runtime DDL: %v", err)
	}
	// The exact crash window: the ALTER committed, the pragma did not.
	if _, err := st.DB.Exec(`ALTER TABLE auction_runtime ADD COLUMN extensions_used INTEGER NOT NULL DEFAULT 0`); err != nil {
		t.Fatalf("pre-applying the ALTER: %v", err)
	}
	setUserVersion(t, st.DB, 1)

	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema on the crash-window file: %v — the table_info probe must make the ALTER a no-op", err)
	}

	var got int
	if err := st.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if got != UserVersion {
		t.Errorf("user_version = %d after a crash-window boot, want %d", got, UserVersion)
	}
}

// TestSchemaNeverLowersANewerFile is research Pitfall 10's guard: the Phase 2
// bootstrap wrote the version unconditionally, which would drag a v2 file back
// to 1 and re-run the migration. A file written by a newer build must be left
// alone — and reported, so the operator sees why the server refused to start.
func TestSchemaNeverLowersANewerFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st := openMigrateStore(t, path)
	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema: %v", err)
	}

	future := UserVersion + 5
	setUserVersion(t, st.DB, future)

	err := Schema(st.DB)
	if err == nil {
		t.Fatal("Schema accepted a file written by a newer schema version; it must refuse to touch it")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Errorf("Schema error = %v, want it to name the schema version conflict", err)
	}

	var got int
	if err := st.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	if got != future {
		t.Errorf("user_version = %d after a refused boot, want the untouched %d", got, future)
	}
}

// ---------------------------------------------------------------------------
// D-74 (Phase 4): the user_version 2→3 step and the server_config KV that
// carries the admin capability. The v1 precedent above is followed exactly — the
// pre-migration file is written as a literal, never derived from schema.go, so
// the test proves the step against a file the previous build could produce.
// ---------------------------------------------------------------------------

// v2AuctionRuntimeDDL is PLAN.md §5's `auction_runtime` as shipped at
// user_version 2: D-50's `extensions_used` column is present, so the v1→v2
// ALTER is history and the 2→3 step is the only migration left to run.
const v2AuctionRuntimeDDL = `
CREATE TABLE auction_runtime (
  item_id         TEXT PRIMARY KEY,
  started_at      INTEGER NOT NULL,
  deadline_at     INTEGER,
  settled_at      INTEGER,
  revision        INTEGER NOT NULL,
  extensions_used INTEGER NOT NULL DEFAULT 0
);`

// hasTable asks sqlite_master — the file's own ground truth — rather than a
// production helper that could agree with itself.
func hasTable(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()

	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatalf("probing sqlite_master for %q: %v", table, err)
	}
	return name == table
}

// isLowerHex reports whether s is made only of 0-9a-f. It is the test's own
// shape check (identity_test.go's is package-private), so a key that came back
// uppercased or with punctuation fails here rather than at the venue.
func isLowerHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// TestMigrationV2ToV3 is D-74's contract: a populated v2 file migrates once to
// user_version 3 with the additive `server_config` table, the row the file
// already had survives, and a key written into the new table is still there
// after a real restart (a new handle on the same file with Schema run again —
// the boot a crash would perform).
func TestMigrationV2ToV3(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st := openMigrateStore(t, path)

	// A v2 file with one populated runtime row and NO server_config table.
	if _, err := st.DB.Exec(v2AuctionRuntimeDDL); err != nil {
		t.Fatalf("writing the v2 auction_runtime DDL: %v", err)
	}
	if _, err := st.DB.Exec(
		`INSERT INTO auction_runtime (item_id, started_at, deadline_at, settled_at, revision, extensions_used)
		 VALUES ('it-v2', 111, 222, NULL, 7, 2)`); err != nil {
		t.Fatalf("inserting the pre-migration runtime row: %v", err)
	}
	setUserVersion(t, st.DB, 2)

	if hasTable(t, st.DB, "server_config") {
		t.Fatal("the v2 fixture already carries server_config — the migration test would prove nothing")
	}

	if err := Schema(st.DB); err != nil {
		t.Fatalf("Schema on a v2 file: %v", err)
	}

	t.Run("reports user_version 3", func(t *testing.T) {
		var got int
		if err := st.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
			t.Fatalf("PRAGMA user_version: %v", err)
		}
		if got != 3 {
			t.Errorf("user_version = %d, want the D-74 version 3", got)
		}
	})

	t.Run("creates an empty server_config table", func(t *testing.T) {
		if !hasTable(t, st.DB, "server_config") {
			t.Fatal("server_config does not exist after the 2→3 migration")
		}
		var n int
		if err := st.DB.QueryRow("SELECT COUNT(*) FROM server_config").Scan(&n); err != nil {
			t.Fatalf("counting server_config rows: %v", err)
		}
		if n != 0 {
			t.Errorf("server_config carries %d rows after the migration, want 0", n)
		}
	})

	t.Run("preserves the v2 row it migrated past", func(t *testing.T) {
		var (
			startedAt  int64
			revision   int64
			extensions int
		)
		err := st.DB.QueryRow(
			`SELECT started_at, revision, extensions_used FROM auction_runtime WHERE item_id = 'it-v2'`).
			Scan(&startedAt, &revision, &extensions)
		if err != nil {
			t.Fatalf("reading the v2 row after the migration: %v", err)
		}
		if startedAt != 111 || revision != 7 || extensions != 2 {
			t.Errorf("v2 row = (started_at %d, revision %d, extensions_used %d), want (111, 7, 2)",
				startedAt, revision, extensions)
		}
	})

	// The key is written through the new table, then the file is reopened and a
	// real boot (Open + Schema) is run against it: the capability must be there.
	const stored = "3b1f6c0e9a5d47b28c6f0e1a4d7c9b2e5f8a1c4e7b0d3f6a9c2e5b8d1f4a7c0e"
	if err := st.SetServerConfig(adminKeyRow, stored); err != nil {
		t.Fatalf("persisting the admin key: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("closing the migrated store: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopening the migrated file: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := Schema(reopened.DB); err != nil {
		t.Fatalf("Schema on the reopened file: %v", err)
	}

	t.Run("a stored key survives a reopen", func(t *testing.T) {
		got, found, err := reopened.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig: %v", err)
		}
		if !found {
			t.Fatal("the persisted admin_key row is gone after a reopen")
		}
		if got != stored {
			t.Errorf("admin_key after a reopen = %q, want the stored %q", got, stored)
		}
	})

	t.Run("a second schema pass leaves the version and the row alone", func(t *testing.T) {
		var got int
		if err := reopened.DB.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
			t.Fatalf("PRAGMA user_version: %v", err)
		}
		if got != 3 {
			t.Errorf("user_version = %d after a second pass, want 3", got)
		}
		var n int
		if err := reopened.DB.QueryRow("SELECT COUNT(*) FROM server_config").Scan(&n); err != nil {
			t.Fatalf("counting server_config rows: %v", err)
		}
		if n != 1 {
			t.Errorf("server_config carries %d rows after a second pass, want the 1 persisted key", n)
		}
	})
}

// TestServerConfig is D-74's resolver contract: the narrow KV pair, the
// env → row → mint precedence ladder, the minted key persisted BEFORE it is
// served, and the fail-hard rule that a broken read or write is an error —
// never a silent re-mint that would invalidate the link the presenter is
// holding. The injected getenv/mint make each rung observable without touching
// the process environment or the real entropy source.
func TestServerConfig(t *testing.T) {
	newStore := func(t *testing.T) (*Store, string) {
		t.Helper()

		path := filepath.Join(t.TempDir(), "auction.db")
		st, err := Open(path)
		if err != nil {
			t.Fatalf("Open(%q): %v", path, err)
		}
		t.Cleanup(func() { _ = st.Close() })
		if err := Schema(st.DB); err != nil {
			t.Fatalf("Schema: %v", err)
		}
		return st, path
	}

	const (
		persistedKey = "1111111111111111111111111111111111111111111111111111111111111111"
		envKey       = "2222222222222222222222222222222222222222222222222222222222222222"
		mintedKey    = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)
	var (
		noEnv          = func(string) string { return "" }
		mintMustNotRun = func() (string, error) {
			return "", errors.New("the mint must not run when a higher rung answered")
		}
	)

	t.Run("an absent row is not found, never an empty value", func(t *testing.T) {
		st, _ := newStore(t)

		value, found, err := st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig: %v", err)
		}
		if found {
			t.Errorf("found = true for a row that was never written (value %q)", value)
		}
	})

	t.Run("a stored row round-trips and overwrites in place", func(t *testing.T) {
		st, _ := newStore(t)

		if err := st.SetServerConfig(adminKeyRow, persistedKey); err != nil {
			t.Fatalf("SetServerConfig: %v", err)
		}
		value, found, err := st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig: %v", err)
		}
		if !found || value != persistedKey {
			t.Fatalf("after Set: found = %t, value = %q, want true and %q", found, value, persistedKey)
		}

		if err := st.SetServerConfig(adminKeyRow, mintedKey); err != nil {
			t.Fatalf("SetServerConfig (overwrite): %v", err)
		}
		value, found, err = st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig (after overwrite): %v", err)
		}
		if !found || value != mintedKey {
			t.Errorf("after overwrite: found = %t, value = %q, want true and %q", found, value, mintedKey)
		}

		var n int
		if err := st.DB.QueryRow(`SELECT COUNT(*) FROM server_config`).Scan(&n); err != nil {
			t.Fatalf("counting server_config rows: %v", err)
		}
		if n != 1 {
			t.Errorf("server_config carries %d rows after an overwrite, want the 1 upserted row", n)
		}
	})

	t.Run("the env pin wins over the persisted row", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.SetServerConfig(adminKeyRow, persistedKey); err != nil {
			t.Fatalf("seeding the persisted row: %v", err)
		}

		key, err := st.ResolveAdminKey(func(name string) string {
			if name != adminKeyEnv {
				t.Errorf("getenv(%q), want %q", name, adminKeyEnv)
			}
			return envKey
		}, mintMustNotRun)
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != envKey {
			t.Errorf("key = %q, want the env pin %q", key, envKey)
		}

		// An env pin does not overwrite the durable row: dropping the env var on
		// the next boot must fall back to the same key, not to a fresh one.
		value, found, err := st.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig: %v", err)
		}
		if !found || value != persistedKey {
			t.Errorf("persisted row = (%t, %q) after an env-pinned boot, want it untouched as (%q)", found, value, persistedKey)
		}
	})

	t.Run("an env pin arrives trimmed", func(t *testing.T) {
		st, _ := newStore(t)

		// A shell export or a .env line can carry trailing whitespace; the
		// printed /admin?k=… link is copy-pasted, so a stray newline would
		// produce a link the gate refuses.
		key, err := st.ResolveAdminKey(func(string) string { return "  " + envKey + "\n" }, mintMustNotRun)
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != envKey {
			t.Errorf("key = %q, want the whitespace-trimmed env pin %q", key, envKey)
		}
	})

	t.Run("a blank env falls through to the persisted row", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.SetServerConfig(adminKeyRow, persistedKey); err != nil {
			t.Fatalf("seeding the persisted row: %v", err)
		}

		key, err := st.ResolveAdminKey(func(string) string { return "   " }, mintMustNotRun)
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != persistedKey {
			t.Errorf("key = %q, want the persisted %q", key, persistedKey)
		}
	})

	t.Run("an empty persisted row is absent, not the key", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.SetServerConfig(adminKeyRow, ""); err != nil {
			t.Fatalf("seeding the empty row: %v", err)
		}

		// A row that lost its value must not authorize an empty key; the
		// resolver heals by minting rather than serving "".
		key, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil })
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != mintedKey {
			t.Errorf("key = %q, want a freshly minted key %q", key, mintedKey)
		}
	})

	t.Run("no env and no row mints and persists before returning", func(t *testing.T) {
		st, path := newStore(t)

		key, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil })
		if err != nil {
			t.Fatalf("ResolveAdminKey: %v", err)
		}
		if key != mintedKey {
			t.Errorf("key = %q, want the minted %q", key, mintedKey)
		}
		if len(key) != 64 || !isLowerHex(key) {
			t.Errorf("minted key %q is not 64 lowercase hex characters (the 32 crypto/rand bytes D-74 pins)", key)
		}

		// Durable BEFORE it is served: a fresh handle on the same file — the
		// boot a restart performs — reads the same key back.
		if err := st.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}
		reopened, err := Open(path)
		if err != nil {
			t.Fatalf("reopening the file: %v", err)
		}
		t.Cleanup(func() { _ = reopened.Close() })

		got, found, err := reopened.GetServerConfig(adminKeyRow)
		if err != nil {
			t.Fatalf("GetServerConfig after reopen: %v", err)
		}
		if !found || got != mintedKey {
			t.Errorf("the minted key was not persisted before it was returned: found = %t, value = %q", found, got)
		}
	})

	t.Run("a mint failure is an error and nothing is persisted", func(t *testing.T) {
		st, _ := newStore(t)

		_, err := st.ResolveAdminKey(noEnv, func() (string, error) { return "", errors.New("entropy exhausted") })
		if err == nil {
			t.Fatal("ResolveAdminKey returned no error when the mint failed")
		}
		if !strings.Contains(err.Error(), "entropy exhausted") {
			t.Errorf("error = %v, want it to carry the mint's own failure", err)
		}
		if _, found, gerr := st.GetServerConfig(adminKeyRow); gerr != nil {
			t.Fatalf("GetServerConfig: %v", gerr)
		} else if found {
			t.Error("a key row was persisted although the mint failed")
		}
	})

	t.Run("an empty mint is refused, never served as the key", func(t *testing.T) {
		st, _ := newStore(t)

		_, err := st.ResolveAdminKey(noEnv, func() (string, error) { return "", nil })
		if err == nil {
			t.Fatal("ResolveAdminKey served an empty key — an empty cookie would satisfy Equal")
		}
		if _, found, gerr := st.GetServerConfig(adminKeyRow); gerr != nil {
			t.Fatalf("GetServerConfig: %v", gerr)
		} else if found {
			t.Error("an empty key was persisted")
		}
	})

	t.Run("a closed store is an error, never a silent re-mint", func(t *testing.T) {
		st, _ := newStore(t)
		if err := st.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}

		// The row cannot be read (or written) on a dead handle: booting on
		// anything but an error here would mint a key the file does not hold.
		if _, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil }); err == nil {
			t.Fatal("ResolveAdminKey answered from a closed store instead of failing the boot")
		}
	})

	t.Run("a write failure is an error, never an in-memory key", func(t *testing.T) {
		st, _ := newStore(t)

		// MaxOpenConns(1) puts this pragma on the one connection the resolver
		// uses: the row read still works, the INSERT cannot land.
		if _, err := st.DB.Exec("PRAGMA query_only = 1"); err != nil {
			t.Fatalf("setting query_only: %v", err)
		}

		_, err := st.ResolveAdminKey(noEnv, func() (string, error) { return mintedKey, nil })
		if err == nil {
			t.Fatal("ResolveAdminKey served a key the write could not persist — a restart would invalidate the link")
		}
		if !strings.Contains(err.Error(), "server_config") {
			t.Errorf("error = %v, want it to name the row that could not be written", err)
		}
	})
}
