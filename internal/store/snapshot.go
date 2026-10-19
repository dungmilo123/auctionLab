package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"context"
)

// This file is the parameterized snapshot repository: the only code in the
// project that reads game state out of PostgreSQL or writes a mutation into it.
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
// Phase 3 extends it additively with the item, runtime and bid write sets:
// nothing here may become a second transaction (RT-06).
type Mutation struct {
	Phase         string
	CurrentItemID string
	Revision      int64
	UpdatedAt     int64
	Players       []PlayerRow

	// Phase 4 (D-81/D-84): the persisted configuration, or nil when this
	// mutation did not edit it — exactly like Runtime, because "unchanged" is
	// not "erase" and a reset that leaves the config alone must not clear the
	// selected join URL. Commit writes it into the same game-row UPDATE, in the
	// same transaction as the revision.
	ConfigJSON *string

	// Phase 3 (D-49): the auction write set. Items carries only the rows that
	// changed (the engine diffs), DeletedItems names removed ids, Runtime is nil
	// unless the timing row moved, and Bids is append-only.
	Items        []ItemRow
	DeletedItems []string
	Runtime      *RuntimeRow
	Bids         []BidRow

	// Phase 6 (D-110): the sealed write set — an UPSERT list, in the same
	// transaction as the revision, because `sealed_bids` holds the effective bid
	// of one (item, player) and an edit changes the row rather than appending
	// one. The statement's DO UPDATE deliberately omits `submitted_at`, which is
	// what keeps SEAL-05's tie-break instant frozen (D-108).
	SealedBids []SealedBidRow

	// Phase 4 (D-78): the reset write set. Every Phase 3 field above is
	// growth-only — a nil Runtime means "unchanged", not "removed", and neither
	// the append-only Bids nor the never-diffed sealed bids can express a
	// clearing — so the removals are named explicitly instead of inferred.
	// ClearBids/ClearSealedBids/DeleteRuntime are whole-table clears, because a
	// reset means "no item has played"; DeletedPlayers is an id list, because a
	// player row does have an in-memory identity to list.
	//
	// Commit runs the children (auction_runtime, sealed_bids, bids) before the
	// players: with PostgreSQL foreign keys, a player delete
	// issued first raises a constraint failure that the engine escalates to a
	// process exit (research Pitfall 5).
	ClearBids       bool
	ClearSealedBids bool
	DeleteRuntime   bool
	DeletedPlayers  []string
}

// LoadGame returns the `game` row. A missing row is an error: boot is
// Schema → Bootstrap → LoadGame, so a missing row means the bootstrap did not
// run, and a zero-valued row would look exactly like a legitimate LOBBY.
func (s *Store) LoadGame() (GameRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const query = `
SELECT phase, COALESCE(current_item_id, ''), revision, config_json, updated_at
FROM game WHERE id = 1`

	var g GameRow
	err := s.DB.QueryRowContext(ctx, query).Scan(&g.Phase, &g.CurrentItemID, &g.Revision, &g.ConfigJSON, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return GameRow{}, fmt.Errorf("store: game row missing — Bootstrap must run before LoadGame")
	}
	if err != nil {
		return GameRow{}, sanitize("loading the game row", err)
	}
	return g, nil
}

// LoadPlayers returns every player row in join order (joined_at, then id, so
// the order is total and a restart reproduces the lobby exactly).
func (s *Store) LoadPlayers() ([]PlayerRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const query = `
SELECT id, nickname, nickname_key, token, budget, total_value, joined_at, last_seen_at
FROM players ORDER BY joined_at, id`

	rows, err := s.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, sanitize("loading players", err)
	}
	defer func() { _ = rows.Close() }()

	var players []PlayerRow
	for rows.Next() {
		var p PlayerRow
		if err := rows.Scan(&p.ID, &p.Nickname, &p.NicknameKey, &p.Token, &p.Budget, &p.TotalValue, &p.JoinedAt, &p.LastSeenAt); err != nil {
			return nil, sanitize("scanning a player row", err)
		}
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, sanitize("reading players", err)
	}
	return players, nil
}

// Commit writes one mutation in a single transaction. Every statement is
// parameterized (no string-built SQL), and the rows are upserted by primary
// key, so re-committing an unchanged row is a no-op rather than a conflict.
//
// The `players.nickname_key` UNIQUE index is what makes nickname uniqueness
// atomic (D-28); its violation surfaces as SQLSTATE 23505,
// which the caller translates with IsUniqueViolation.
func (s *Store) Commit(m Mutation) error {
	return sanitize("mutation", s.withWriter(func(ctx context.Context, conn *sql.Conn) error {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return sanitize("beginning a mutation", err)
		}
		// No-op after a successful Commit; guarantees the transaction cannot leak
		// on any error path above.
		defer func() { _ = tx.Rollback() }()

		// The statement order below satisfies PostgreSQL foreign keys for the whole
		// write set: players (referenced by bids), then items (referenced by the
		// game row's current_item_id, the runtime row and bids), then the game row
		// update (which may point at a row created in this very mutation), then the
		// runtime deletes (a runtime row references the item it belongs to), then the
		// item deletes (safe only after the game row stopped referencing the old
		// item), then the D-78 reset deletes children-first — sealed_bids, bids and
		// auction_runtime all precede the players they reference — and finally the
		// runtime row, then the append-only bids.
		const upsertPlayer = `
INSERT INTO players (id, nickname, nickname_key, token, budget, total_value, joined_at, last_seen_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT(id) DO UPDATE SET
  nickname     = excluded.nickname,
  nickname_key = excluded.nickname_key,
  token        = excluded.token,
  budget       = excluded.budget,
  total_value  = excluded.total_value,
  joined_at    = excluded.joined_at,
  last_seen_at = excluded.last_seen_at`
		for _, p := range m.Players {
			if _, err := tx.ExecContext(ctx, upsertPlayer, p.ID, p.Nickname, p.NicknameKey, p.Token, p.Budget, p.TotalValue, p.JoinedAt, p.LastSeenAt); err != nil {
				return fmt.Errorf("store: upserting player %q: %w", p.ID, err)
			}
		}

		// Phase 3 (D-49): the auction write set rides the SAME transaction.
		const upsertItem = `
INSERT INTO items (id, order_index, name, description, image_url, auction_type, truth_value,
                   hint_min, hint_max, start_price, min_increment, floor_price, decrement_step,
                   decrement_interval_ms, duration_ms, anti_snipe_mode, anti_snipe_window_ms,
                   max_extensions, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
ON CONFLICT(id) DO UPDATE SET
  order_index           = excluded.order_index,
  name                  = excluded.name,
  description           = excluded.description,
  image_url             = excluded.image_url,
  auction_type          = excluded.auction_type,
  truth_value           = excluded.truth_value,
  hint_min              = excluded.hint_min,
  hint_max              = excluded.hint_max,
  start_price           = excluded.start_price,
  min_increment         = excluded.min_increment,
  floor_price           = excluded.floor_price,
  decrement_step        = excluded.decrement_step,
  decrement_interval_ms = excluded.decrement_interval_ms,
  duration_ms           = excluded.duration_ms,
  anti_snipe_mode       = excluded.anti_snipe_mode,
  anti_snipe_window_ms  = excluded.anti_snipe_window_ms,
  max_extensions        = excluded.max_extensions,
  status                = excluded.status`
		for _, it := range m.Items {
			if _, err := tx.ExecContext(ctx, upsertItem,
				it.ID, it.OrderIndex, it.Name, it.Description, it.ImageURL, it.AuctionType, it.TruthValue,
				nullableInt64(it.HintMin), nullableInt64(it.HintMax), it.StartPrice,
				nullableInt64(it.MinIncrement), nullableInt64(it.FloorPrice), nullableInt64(it.DecrementStep),
				nullableInt64(it.DecrementIntervalMS), nullableInt64(it.DurationMS), it.AntiSnipeMode,
				nullableInt64(it.AntiSnipeWindowMS), nullableInt64(it.MaxExtensions), it.Status,
			); err != nil {
				return fmt.Errorf("store: upserting item %q: %w", it.ID, err)
			}
		}

		const updateGame = `
UPDATE game SET phase = $1, current_item_id = $2, revision = $3, updated_at = $4,
                config_json = COALESCE($5, config_json) WHERE id = 1`
		res, err := tx.ExecContext(ctx, updateGame, m.Phase, nullableItemID(m.CurrentItemID), m.Revision, m.UpdatedAt, nullableConfigJSON(m.ConfigJSON))
		if err != nil {
			return sanitize("updating the game row", err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return sanitize("checking the game row update", err)
		}
		if affected == 0 {
			return fmt.Errorf("store: game row missing — Bootstrap must run before the first mutation")
		}

		// Phase 6 (D-108/D-110): the effective sealed bids, in the SAME transaction
		// as the revision. The statement runs after the player and item rows it
		// references are in and before every delete below, and its DO UPDATE
		// deliberately omits `submitted_at`: that omission IS the immutability rule
		// SEAL-05's tie-break depends on. The list is an upsert write set, so
		// re-committing an unchanged row is a no-op.
		const upsertSealedBid = `
INSERT INTO sealed_bids (item_id, player_id, amount, submitted_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(item_id, player_id) DO UPDATE SET
  amount     = excluded.amount,
  updated_at = excluded.updated_at`
		for _, b := range m.SealedBids {
			if _, err := tx.ExecContext(ctx, upsertSealedBid, b.ItemID, b.PlayerID, b.Amount, b.SubmittedAt, b.UpdatedAt); err != nil {
				return fmt.Errorf("store: upserting the sealed bid for item %q player %q: %w", b.ItemID, b.PlayerID, err)
			}
		}

		// D-78: the reset half of the write set. A runtime row is a child of the item
		// it belongs to, so the removal runs before the item deletes.
		if m.DeleteRuntime {
			if _, err := tx.ExecContext(ctx, `DELETE FROM auction_runtime`); err != nil {
				return sanitize("clearing the auction_runtime rows", err)
			}
		}

		const deleteItem = `DELETE FROM items WHERE id = $1`
		for _, id := range m.DeletedItems {
			if _, err := tx.ExecContext(ctx, deleteItem, id); err != nil {
				return fmt.Errorf("store: deleting item %q: %w", id, err)
			}
		}

		// D-78: sealed bids and bids both reference players, so the clears run before
		// any player delete. `DELETE FROM players` with a live child row raises
		// `foreign key constraint failed`, which the engine escalates through its
		// fatal hook to a process exit (T-04-05) — the order here is the mitigation,
		// and reset_test.go's negative control pins it.
		if m.ClearSealedBids {
			if _, err := tx.ExecContext(ctx, `DELETE FROM sealed_bids`); err != nil {
				return sanitize("clearing the sealed_bids rows", err)
			}
		}
		if m.ClearBids {
			if _, err := tx.ExecContext(ctx, `DELETE FROM bids`); err != nil {
				return sanitize("clearing the bids rows", err)
			}
		}
		const deletePlayer = `DELETE FROM players WHERE id = $1`
		for _, id := range m.DeletedPlayers {
			if _, err := tx.ExecContext(ctx, deletePlayer, id); err != nil {
				return fmt.Errorf("store: deleting player %q: %w", id, err)
			}
		}

		if m.Runtime != nil {
			const upsertRuntime = `
INSERT INTO auction_runtime (item_id, started_at, deadline_at, settled_at, revision, extensions_used)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(item_id) DO UPDATE SET
  started_at      = excluded.started_at,
  deadline_at     = excluded.deadline_at,
  settled_at      = excluded.settled_at,
  revision        = excluded.revision,
  extensions_used = excluded.extensions_used`
			if _, err := tx.ExecContext(ctx, upsertRuntime,
				m.Runtime.ItemID, m.Runtime.StartedAt, nullableInt64(m.Runtime.DeadlineAt),
				nullableInt64(m.Runtime.SettledAt), m.Runtime.Revision, m.Runtime.ExtensionsUsed,
			); err != nil {
				return fmt.Errorf("store: upserting the runtime row for item %q: %w", m.Runtime.ItemID, err)
			}
		}

		// Bids are append-only (D-51): a row is only ever inserted, never updated.
		const insertBid = `INSERT INTO bids (item_id, player_id, amount, kind, created_at) VALUES ($1, $2, $3, $4, $5)`
		for _, b := range m.Bids {
			if _, err := tx.ExecContext(ctx, insertBid, b.ItemID, b.PlayerID, b.Amount, b.Kind, b.CreatedAt); err != nil {
				return fmt.Errorf("store: appending a bid for item %q: %w", b.ItemID, err)
			}
		}

		if err := tx.Commit(); err != nil {
			return sanitize("committing a mutation", err)
		}
		return nil
	}))
}

// IsUniqueViolation distinguishes PostgreSQL uniqueness from FK/check failures.
func IsUniqueViolation(err error) bool { return SQLState(err) == "23505" }

// nullableItemID turns the empty string into SQL NULL: current_item_id is a
// nullable foreign key (PLAN.md §5), and an empty string would be a value.
func nullableItemID(id string) any {
	if id == "" {
		return nil
	}
	return id
}

// nullableConfigJSON turns the absent pointer into SQL NULL so the UPDATE's
// COALESCE keeps the committed value: a nil ConfigJSON means "this mutation did
// not edit the configuration", never "erase it".
func nullableConfigJSON(raw *string) any {
	if raw == nil {
		return nil
	}
	return *raw
}
