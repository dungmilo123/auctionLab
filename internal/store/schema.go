package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// UserVersion is D-36's schema version, raised to 2 by D-50, to 3 by D-74 and
// to 4 by D-02: Phase 3 adds `auction_runtime.extensions_used` because the
// anti-snipe cap must survive a crash exactly (deriving it from deadline
// arithmetic does not reconstruct the number of extends), Phase 4 adds the
// `server_config` KV table the boot admin capability persists into, and the
// currency migration rewrites the persisted display currency to USD. The
// bootstrap stays one idempotent DDL pass plus one guarded migration —
// deliberately no migration framework, because a demo database is disposable
// (delete auction.db to reset).
const UserVersion = 4

// gameConfigJSON is the bootstrapped game row's default configuration (D-36 /
// PLAN.md §5: starting_budget 1.000.000). It is written by the schema
// bootstrap, so this is the one place the default lives.
//
// D-02: the currency is USD. The amount itself is deliberately unchanged — the
// stored integers are the game's own accounting, not a converted money value,
// and the button/caption conversion happens in the browser's ONE formatter.
const gameConfigJSON = `{"starting_budget":1000000,"currency":"USD"}`

// The two currency literals D-02 knows: the value older files were seeded with,
// and the one this build writes. They are compared, never rendered — the
// admin projection carries whatever the file holds.
const (
	currencyVND = "VND"
	currencyUSD = "USD"
)

// schemaDDL is PLAN.md §5 in full, plus D-28's nickname_key column and its
// UNIQUE index. Every statement is CREATE … IF NOT EXISTS: the pass runs on
// every boot, and the second run must change nothing.
//
// `nickname` keeps PLAN §5's plain UNIQUE constraint; `nickname_key` carries
// the case-insensitive one that actually decides (SQLite's NOCASE folds ASCII
// only, which is not enough for Vietnamese names — D-28).
//
// One Exec, all statements: SQLite applies a multi-statement script
// atomically enough for boot, and the research probe verified the whole set is
// idempotent in a single call.
const schemaDDL = `
CREATE TABLE IF NOT EXISTS players (
  id           TEXT PRIMARY KEY,
  nickname     TEXT NOT NULL UNIQUE,
  nickname_key TEXT NOT NULL,
  token        TEXT NOT NULL UNIQUE,
  budget       INTEGER NOT NULL,
  total_value  INTEGER NOT NULL DEFAULT 0,
  joined_at    INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_players_nickname_key ON players(nickname_key);

CREATE TABLE IF NOT EXISTS items (
  id                    TEXT PRIMARY KEY,
  order_index           INTEGER NOT NULL,
  name                  TEXT NOT NULL,
  description           TEXT,
  image_url             TEXT,
  auction_type          TEXT NOT NULL,
  truth_value           INTEGER NOT NULL,
  hint_min              INTEGER,
  hint_max              INTEGER,
  start_price           INTEGER NOT NULL,
  min_increment         INTEGER,
  floor_price           INTEGER,
  decrement_step        INTEGER,
  decrement_interval_ms INTEGER,
  duration_ms           INTEGER,
  anti_snipe_mode       TEXT DEFAULT 'disabled',
  anti_snipe_window_ms  INTEGER,
  max_extensions        INTEGER,
  status                TEXT NOT NULL DEFAULT 'pending'
);

CREATE TABLE IF NOT EXISTS sealed_bids (
  item_id      TEXT NOT NULL REFERENCES items(id),
  player_id    TEXT NOT NULL REFERENCES players(id),
  amount       INTEGER NOT NULL,
  submitted_at INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  PRIMARY KEY (item_id, player_id)
);

CREATE TABLE IF NOT EXISTS bids (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  item_id    TEXT NOT NULL REFERENCES items(id),
  player_id  TEXT NOT NULL REFERENCES players(id),
  amount     INTEGER NOT NULL,
  kind       TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS auction_runtime (
  item_id         TEXT PRIMARY KEY REFERENCES items(id),
  started_at      INTEGER NOT NULL,
  deadline_at     INTEGER,
  settled_at      INTEGER,
  revision        INTEGER NOT NULL,
  extensions_used INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS game (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  phase           TEXT NOT NULL,
  current_item_id TEXT REFERENCES items(id),
  revision        INTEGER NOT NULL,
  config_json     TEXT NOT NULL,
  updated_at      INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  type         TEXT NOT NULL,
  payload_json TEXT,
  created_at   INTEGER NOT NULL
);
`

// serverConfigDDL is D-74's additive key/value table. It is deliberately NOT
// part of schemaDDL above: PLAN.md §5 is what schemaDDL transcribes, and the
// version gate below is what creates this table, exactly as the v1→v2 ALTER is
// what creates `extensions_used`. `key` is a legal SQLite column name (probed
// against this driver) and is D-74's own vocabulary.
const serverConfigDDL = `
CREATE TABLE IF NOT EXISTS server_config (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);`

// Schema applies the complete schema (D-36) and brings the file to
// UserVersion. It is idempotent: a second call on an existing file changes
// nothing and still reports the same version.
//
// It never LOWERS a version. The Phase 2 bootstrap wrote the pragma
// unconditionally, which would drag a v2 file back to 1 and re-run the
// migration; a file written by a newer build is refused outright instead
// (research Pitfall 10).
//
// This is the only place in the project where DDL or PRAGMA statements may
// live (research Pitfall 12): the mutation path executes no schema work.
func Schema(db *sql.DB) error {
	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("store: reading user_version: %w", err)
	}
	if current > UserVersion {
		return fmt.Errorf("store: schema version %d is newer than this build supports (%d) — refusing to touch the file", current, UserVersion)
	}

	if _, err := db.Exec(schemaDDL); err != nil {
		return fmt.Errorf("store: applying schema: %w", err)
	}

	if current < UserVersion {
		if err := migrate(db); err != nil {
			return err
		}
	}

	var got int
	if err := db.QueryRow("PRAGMA user_version").Scan(&got); err != nil {
		return fmt.Errorf("store: reading user_version: %w", err)
	}
	if got != UserVersion {
		return fmt.Errorf("store: user_version is %d after the bootstrap, want %d", got, UserVersion)
	}
	return nil
}

// migrate brings a v1 (or brand-new) file to UserVersion. Two properties are
// the contract (D-50, research §1–2):
//
//   - the ALTER/CREATE and `PRAGMA user_version` commit in ONE transaction, so a
//     crash can never leave the schema and the version disagreeing about each
//     other;
//   - every step is guarded by a probe, because a boot that crashed exactly
//     between the change and the pragma leaves a file where the change exists
//     but the version does not — a version-only guard would re-issue it and fail
//     forever with "duplicate column name" (a plain error, not a UNIQUE
//     violation, so D-34 would escalate it to a process exit).
//
// A v1 file runs both steps; a v2 file only the 2→3 step.
func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: beginning the schema migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// v1 → v2 (D-50): the anti-snipe cap column. `PRAGMA table_info` is the
	// probe (see the doc comment above).
	has, err := columnExists(tx, "auction_runtime", "extensions_used")
	if err != nil {
		return err
	}
	if !has {
		if _, err := tx.Exec(`ALTER TABLE auction_runtime ADD COLUMN extensions_used INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("store: adding auction_runtime.extensions_used: %w", err)
		}
	}

	// v2 → v3 (D-74): the server_config KV the boot admin capability persists
	// into. `CREATE TABLE IF NOT EXISTS` is its own crash-window probe — the
	// statement is safe to re-issue, which is why no separate table_info check is
	// needed here.
	if _, err := tx.Exec(serverConfigDDL); err != nil {
		return fmt.Errorf("store: creating the server_config table: %w", err)
	}

	// v3 → v4 (D-02): the persisted display currency becomes USD. The step is
	// its own crash-window probe for the same reason the CREATE above is: it
	// rewrites only a file that still carries the old literal, so re-running it
	// (a boot that died before the pragma) changes nothing the second time.
	if err := migrateCurrencyToUSD(tx); err != nil {
		return err
	}

	// PRAGMA cannot be parameterized; UserVersion is a compile-time constant.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", UserVersion)); err != nil {
		return fmt.Errorf("store: setting user_version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: committing the schema migration: %w", err)
	}
	return nil
}

// migrateCurrencyToUSD is D-02's data step: every file written before this
// build persisted `"currency":"VND"` (the bootstrap literal), and the whole
// display surface now renders USD, so the stored value is rewritten in place.
//
// Two deliberate properties:
//
//   - NO amounts are rescaled. `starting_budget` and every balance stay the
//     same integers — they are the game's own accounting units, and the plan's
//     USD surface is a display conversion, not a re-denomination. Rewriting the
//     numbers would silently move budgets mid-game (D-02).
//   - A file that already says USD, an unknown future literal, or no game row at
//     all (a fresh file — Bootstrap writes the row later) is left untouched.
//     The rewrite is decoded and re-encoded as a map rather than string-edited,
//     so a config_json carrying unknown keys survives it byte-for-byte in
//     meaning.
func migrateCurrencyToUSD(tx *sql.Tx) error {
	var raw string
	err := tx.QueryRow(`SELECT config_json FROM game WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store: reading the game row for the currency migration: %w", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return fmt.Errorf("store: parsing the persisted config_json %q: %w", raw, err)
	}
	if cfg["currency"] != currencyVND {
		return nil
	}
	cfg["currency"] = currencyUSD

	updated, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("store: re-encoding the migrated config_json: %w", err)
	}
	if _, err := tx.Exec(`UPDATE game SET config_json = ? WHERE id = 1`, string(updated)); err != nil {
		return fmt.Errorf("store: writing the migrated config_json: %w", err)
	}
	return nil
}

// columnExists probes one table's columns. The names here are compile-time
// literals, never caller input, so the concatenation cannot be injected into.
func columnExists(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, fmt.Errorf("store: probing %s columns: %w", table, err)
	}

	found := false
	for rows.Next() {
		var (
			cid, notNull, pk int
			name, typ        string
			dflt             any
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("store: scanning %s columns: %w", table, err)
		}
		if name == column {
			found = true
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return false, fmt.Errorf("store: reading %s columns: %w", table, err)
	}
	return found, nil
}

// Bootstrap inserts the single `game` row if it does not exist (D-36). The
// phase literal is the frozen wire value (game.PhaseLobby); the engine parses
// it into a game.Phase on load.
func Bootstrap(db *sql.DB, now time.Time) error {
	const insert = `
INSERT INTO game (id, phase, current_item_id, revision, config_json, updated_at)
VALUES (1, 'LOBBY', NULL, 0, ?, ?)
ON CONFLICT(id) DO NOTHING`

	if _, err := db.Exec(insert, gameConfigJSON, now.UnixMilli()); err != nil {
		return fmt.Errorf("store: bootstrapping the game row: %w", err)
	}
	return nil
}
