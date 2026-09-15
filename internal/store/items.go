package store

import (
	"errors"
	"fmt"
)

// This file is the Phase 3 item/runtime/bid repository: the row types and the
// load queries that make the auction half of one mutation durable (D-49/D-50,
// RT-06). It mirrors snapshot.go's discipline — parameterless ordered SELECTs,
// rows.Err() checked, every error wrapped with "store: …" — and keeps all
// schema work out of the mutation path (research Pitfall 12).

// errREDPhase3Store is the RED-phase scaffold of plan 03-01: the loaders below
// return it until the GREEN commit implements them. It exists only so the
// failing tests compile as tests (a load failure would be INVALID_RED, #3770).
var errREDPhase3Store = errors.New("store: Phase 3 loaders are not implemented yet")

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
// row per accepted bid (kind = "bid"); settle itself writes only
// auction_runtime.settled_at.
type BidRow struct {
	ID        int64
	ItemID    string
	PlayerID  string
	Amount    int64
	Kind      string
	CreatedAt int64
}

// LoadItems returns every item row in presentation order (order_index, then id,
// so the order is total and a restart reproduces the list exactly).
func (s *Store) LoadItems() ([]ItemRow, error) {
	return nil, fmt.Errorf("store: LoadItems: %w", errREDPhase3Store)
}

// LoadRuntime returns the runtime row of one item. The bool distinguishes "no
// row" (an item that has never gone live — legitimate outside LIVE) from a
// zero-valued row, which would look exactly like `started_at = 0`.
func (s *Store) LoadRuntime(itemID string) (RuntimeRow, bool, error) {
	return RuntimeRow{}, false, fmt.Errorf("store: LoadRuntime: %w", errREDPhase3Store)
}

// LoadBids returns every bid row in append order (id, then nothing — the id IS
// the order).
func (s *Store) LoadBids() ([]BidRow, error) {
	return nil, fmt.Errorf("store: LoadBids: %w", errREDPhase3Store)
}

// nullableInt64 turns the row types' pointers into the driver's NULL contract.
func nullableInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
