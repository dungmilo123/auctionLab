package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// This file is the Phase 3 item/runtime/bid repository: the row types and the
// load queries that make the auction half of one mutation durable (D-49/D-50,
// RT-06). It mirrors snapshot.go's discipline — ordered SELECTs, rows.Err()
// checked, every error wrapped with "store: …" — and keeps all schema work out
// of the mutation path (research Pitfall 12).

// ItemRow is one row of the `items` table (PLAN.md §5). Nullable columns are
// pointers so SQL NULL and a legitimate 0 stay distinguishable: `hint_min` may
// be 0, and `max_extensions` NULL means "no cap" (D-59).
type ItemRow struct {
	ID                  string
	OrderIndex          int64
	Name                string
	Description         string
	ImageURL            string
	AuctionType         string
	TruthValue          int64
	HintMin             *int64
	HintMax             *int64
	StartPrice          int64
	MinIncrement        *int64
	FloorPrice          *int64
	DecrementStep       *int64
	DecrementIntervalMS *int64
	DurationMS          *int64
	AntiSnipeMode       string
	AntiSnipeWindowMS   *int64
	MaxExtensions       *int64
	Status              string
}

// RuntimeRow is one row of the `auction_runtime` table — the persisted timing
// of one item's run. `extensions_used` is D-50's column: the cap must survive a
// crash exactly, so it is stored, never derived from deadline arithmetic.
type RuntimeRow struct {
	ItemID         string
	StartedAt      int64
	DeadlineAt     *int64 // NULL for an item type with no deadline
	SettledAt      *int64 // NULL until settled
	Revision       int64
	ExtensionsUsed int
}

// BidRow is one row of the append-only `bids` audit (D-51). Phase 3 writes one
// row per accepted bid (kind = "bid"); Phase 5 adds one `accept` per Dutch
// acceptance (D-98), and Phase 6 one `sealed_edit` per sealed submit or edit
// (D-109). Settle itself writes only auction_runtime.settled_at.
type BidRow struct {
	ID        int64
	ItemID    string
	PlayerID  string
	Amount    int64
	Kind      string
	CreatedAt int64
}

// SealedBidRow is one row of the `sealed_bids` table (PLAN.md §5) — the
// EFFECTIVE sealed bid of one player on one item (D-108). All five columns are
// NOT NULL, so every field is a plain value: `submitted_at` is the FIRST
// submission's instant and never moves, `updated_at` is the latest edit's.
type SealedBidRow struct {
	ItemID      string
	PlayerID    string
	Amount      int64
	SubmittedAt int64
	UpdatedAt   int64
}

// LoadItems returns every item row in presentation order (order_index, then id,
// so the order is total and a restart reproduces the list exactly).
func (s *Store) LoadItems() ([]ItemRow, error) {
	const query = `
SELECT id, order_index, name, COALESCE(description, ''), COALESCE(image_url, ''),
       auction_type, truth_value, hint_min, hint_max, start_price, min_increment, floor_price,
       decrement_step, decrement_interval_ms, duration_ms, COALESCE(anti_snipe_mode, ''),
       anti_snipe_window_ms, max_extensions, status
FROM items ORDER BY order_index, id`

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("store: loading items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []ItemRow
	for rows.Next() {
		var it ItemRow
		// The nullable columns scan into *int64 fields: database/sql allocates
		// for a value and leaves nil for SQL NULL (probed against this driver).
		if err := rows.Scan(
			&it.ID, &it.OrderIndex, &it.Name, &it.Description, &it.ImageURL,
			&it.AuctionType, &it.TruthValue, &it.HintMin, &it.HintMax, &it.StartPrice,
			&it.MinIncrement, &it.FloorPrice, &it.DecrementStep, &it.DecrementIntervalMS,
			&it.DurationMS, &it.AntiSnipeMode, &it.AntiSnipeWindowMS, &it.MaxExtensions, &it.Status,
		); err != nil {
			return nil, fmt.Errorf("store: scanning an item row: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: reading items: %w", err)
	}
	return items, nil
}

// LoadRuntime returns the runtime row of one item. The bool distinguishes "no
// row" (an item that has never gone live — legitimate outside LIVE) from a
// zero-valued row, which would look exactly like `started_at = 0`.
func (s *Store) LoadRuntime(itemID string) (RuntimeRow, bool, error) {
	const query = `
SELECT item_id, started_at, deadline_at, settled_at, revision, extensions_used
FROM auction_runtime WHERE item_id = ?`

	var rt RuntimeRow
	err := s.DB.QueryRow(query, itemID).Scan(
		&rt.ItemID, &rt.StartedAt, &rt.DeadlineAt, &rt.SettledAt, &rt.Revision, &rt.ExtensionsUsed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeRow{}, false, nil
	}
	if err != nil {
		return RuntimeRow{}, false, fmt.Errorf("store: loading the runtime row for item %q: %w", itemID, err)
	}
	return rt, true, nil
}

// LoadBids returns every bid row in append order: the AUTOINCREMENT id IS the
// order, so a restart reproduces the trail exactly (research §4 uses this to
// reconstruct the running item's top bid).
func (s *Store) LoadBids() ([]BidRow, error) {
	const query = `
SELECT id, item_id, player_id, amount, kind, created_at FROM bids ORDER BY id`

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("store: loading bids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var bids []BidRow
	for rows.Next() {
		var b BidRow
		if err := rows.Scan(&b.ID, &b.ItemID, &b.PlayerID, &b.Amount, &b.Kind, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scanning a bid row: %w", err)
		}
		bids = append(bids, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: reading bids: %w", err)
	}
	return bids, nil
}

// nullableInt64 turns the row types' pointers into the driver's NULL contract:
// nil is SQL NULL, a pointer is its value — including a legitimate 0.
func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// LoadSealedBids returns every effective sealed bid in a TOTAL order —
// item_id, then the FIRST submission instant, then player id — so a restart
// reproduces the slice exactly and the tie-break reads the same sequence the
// live process built (D-108/D-113).
func (s *Store) LoadSealedBids() ([]SealedBidRow, error) {
	const query = `
SELECT item_id, player_id, amount, submitted_at, updated_at
FROM sealed_bids ORDER BY item_id, submitted_at, player_id`

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("store: loading sealed bids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sealed []SealedBidRow
	for rows.Next() {
		var b SealedBidRow
		if err := rows.Scan(&b.ItemID, &b.PlayerID, &b.Amount, &b.SubmittedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scanning a sealed bid row: %w", err)
		}
		sealed = append(sealed, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: reading sealed bids: %w", err)
	}
	return sealed, nil
}
