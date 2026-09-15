package store

import (
	"database/sql"
	"errors"
	"fmt"

	sqlite "modernc.org/sqlite"
)

// This file is the parameterized snapshot repository: the only code in the
// project that reads game state out of SQLite or writes a mutation into it.
// Schema statements live exclusively in schema.go (research Pitfall 12: no
// schema work inside the mutation path).

// GameRow is the single row of the `game` table (PLAN.md §5, id = 1).
type GameRow struct {
	Phase         string // wire phase name, e.g. "LOBBY"
	CurrentItemID string // empty when no item is current
	Revision      int64  // the one canonical game revision (D-35)
	ConfigJSON    string // starting_budget, currency, …
	UpdatedAt     int64  // server clock, milliseconds
}

// PlayerRow is one row of the `players` table (PLAN.md §5).
type PlayerRow struct {
	ID          string
	Nickname    string // display form, normalized (D-27), case preserved
	NicknameKey string // strings.ToLower(Nickname), the UNIQUE key (D-28)
	Token       string // 64 lowercase hex chars, crypto/rand (D-29)
	Budget      int64
	TotalValue  int64
	JoinedAt    int64
	LastSeenAt  int64
}

// Mutation is the write set of one engine mutation: the game row plus every
// player row, committed in ONE transaction together with game.revision (D-34).
// Phase 3 extends it additively with item and bid write sets.
type Mutation struct {
	Phase         string
	CurrentItemID string
	Revision      int64
	UpdatedAt     int64
	Players       []PlayerRow
}

// LoadGame returns the `game` row. A missing row is an error: boot is
// Schema → Bootstrap → LoadGame, so a missing row means the bootstrap did not
// run, and a zero-valued row would look exactly like a legitimate LOBBY.
func (s *Store) LoadGame() (GameRow, error) {
	const query = `
SELECT phase, COALESCE(current_item_id, ''), revision, config_json, updated_at
FROM game WHERE id = 1`

	var g GameRow
	err := s.DB.QueryRow(query).Scan(&g.Phase, &g.CurrentItemID, &g.Revision, &g.ConfigJSON, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GameRow{}, fmt.Errorf("store: game row missing — Bootstrap must run before LoadGame")
	}
	if err != nil {
		return GameRow{}, fmt.Errorf("store: loading the game row: %w", err)
	}
	return g, nil
}

// LoadPlayers returns every player row in join order (joined_at, then id, so
// the order is total and a restart reproduces the lobby exactly).
func (s *Store) LoadPlayers() ([]PlayerRow, error) {
	const query = `
SELECT id, nickname, nickname_key, token, budget, total_value, joined_at, last_seen_at
FROM players ORDER BY joined_at, id`

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("store: loading players: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var players []PlayerRow
	for rows.Next() {
		var p PlayerRow
		if err := rows.Scan(&p.ID, &p.Nickname, &p.NicknameKey, &p.Token, &p.Budget, &p.TotalValue, &p.JoinedAt, &p.LastSeenAt); err != nil {
			return nil, fmt.Errorf("store: scanning a player row: %w", err)
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: reading players: %w", err)
	}
	return players, nil
}

// Commit writes one mutation in a single transaction. Every statement is
// parameterized (no string-built SQL), and the rows are upserted by primary
// key, so re-committing an unchanged row is a no-op rather than a conflict.
//
// The `players.nickname_key` UNIQUE index is what makes nickname uniqueness
// atomic (D-28); its violation surfaces as a *sqlite.Error with code 2067,
// which the caller translates with IsUniqueViolation.
func (s *Store) Commit(m Mutation) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("store: beginning a mutation: %w", err)
	}
	// No-op after a successful Commit; guarantees the transaction cannot leak
	// on any error path above.
	defer func() { _ = tx.Rollback() }()

	const updateGame = `
UPDATE game SET phase = ?, current_item_id = ?, revision = ?, updated_at = ? WHERE id = 1`
	res, err := tx.Exec(updateGame, m.Phase, nullableItemID(m.CurrentItemID), m.Revision, m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: updating the game row: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: checking the game row update: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("store: game row missing — Bootstrap must run before the first mutation")
	}

	const upsertPlayer = `
INSERT INTO players (id, nickname, nickname_key, token, budget, total_value, joined_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  nickname     = excluded.nickname,
  nickname_key = excluded.nickname_key,
  token        = excluded.token,
  budget       = excluded.budget,
  total_value  = excluded.total_value,
  joined_at    = excluded.joined_at,
  last_seen_at = excluded.last_seen_at`
	for _, p := range m.Players {
		if _, err := tx.Exec(upsertPlayer, p.ID, p.Nickname, p.NicknameKey, p.Token, p.Budget, p.TotalValue, p.JoinedAt, p.LastSeenAt); err != nil {
			return fmt.Errorf("store: upserting player %q: %w", p.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: committing a mutation: %w", err)
	}
	return nil
}

// IsUniqueViolation reports whether err is a SQLite UNIQUE-constraint failure.
//
// The numeric code (2067 = SQLITE_CONSTRAINT_UNIQUE) is the contract, never the
// message text: a driver upgrade may reword "UNIQUE constraint failed", and a
// rejection that silently degrades into an opaque error is a 500 in front of a
// classroom (research §4).
func IsUniqueViolation(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == 2067
	}
	return false
}

// nullableItemID turns the empty string into SQL NULL: current_item_id is a
// nullable foreign key (PLAN.md §5), and an empty string would be a value.
func nullableItemID(id string) any {
	if id == "" {
		return nil
	}
	return id
}
