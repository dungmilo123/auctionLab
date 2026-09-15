package game

import (
	"errors"
	"fmt"
)

// This file is the presenter's item surface (ADMN-01/ADMN-02, D-66/D-65): the
// `admin.item.*` payload validation, the create/update/delete/reorder mutations
// and the phase-control commands that drive the state machine.
//
// Every command follows the engine's one path: validate against the committed
// state under the exclusive lock, mutate a scratch copy, commit ONE write set,
// swap, then record the typed events for the lock-held fan-out. A rejection
// returns an error and changes nothing — the hub maps it to a private
// `bad_message` (H12).

// errREDPhase3Items is the RED-phase scaffold of plan 03-01's third task: the
// admin mutations refuse to run until the GREEN commit implements them. It
// exists only so the failing tests compile as tests (a load failure would be
// INVALID_RED, #3770).
var errREDPhase3Items = errors.New("game: the item commands are not implemented yet")

// ItemInput is the `admin.item.upsert` payload (D-66): the full item object, so
// the admin editor round-trips exactly the row it was shown. An empty ID means
// "create with a server-minted id" (H14); a non-empty ID must name an existing
// item or the command is refused.
//
// Every nullable numeric is a pointer so "absent" and a legitimate 0 stay
// distinguishable: `max_extensions: null` means no cap (D-59).
type ItemInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ImageURL    string `json:"image_url"`

	AuctionType string `json:"auction_type"`
	TruthValue  int64  `json:"truth_value"`

	HintMin    *int64 `json:"hint_min"`
	HintMax    *int64 `json:"hint_max"`
	StartPrice int64  `json:"start_price"`

	MinIncrement        *int64 `json:"min_increment"`
	FloorPrice          *int64 `json:"floor_price"`
	DecrementStep       *int64 `json:"decrement_step"`
	DecrementIntervalMS *int64 `json:"decrement_interval_ms"`
	DurationMS          *int64 `json:"duration_ms"`

	AntiSnipeMode     string `json:"anti_snipe_mode"`
	AntiSnipeWindowMS *int64 `json:"anti_snipe_window_ms"`
	MaxExtensions     *int64 `json:"max_extensions"`
}

// ItemUpsert creates (empty ID) or rewrites (known ID) one item (D-66). Only a
// pending or intro item may be rewritten: a live or settled row is history.
func (e *Engine) ItemUpsert(in ItemInput) error {
	return fmt.Errorf("game: ItemUpsert: %w", errREDPhase3Items)
}

// DeleteItem removes one pending or intro item. The current item cannot be
// deleted — the presenter skips it first, so the phase invariants stay intact.
func (e *Engine) DeleteItem(itemID string) error {
	return fmt.Errorf("game: DeleteItem: %w", errREDPhase3Items)
}

// ReorderItems applies a complete unique permutation of the existing item ids
// and rewrites order_index accordingly. Anything else is refused with no
// partial write (D-66).
func (e *Engine) ReorderItems(ids []string) error {
	return fmt.Errorf("game: ReorderItems: %w", errREDPhase3Items)
}

// StartItem is `admin.item.start {id}` (D-65): from LOBBY, SETTLED or FINALE it
// opens the item's intro; from ITEM_INTRO with the same id it goes LIVE.
func (e *Engine) StartItem(itemID string) error {
	return fmt.Errorf("game: StartItem: %w", errREDPhase3Items)
}

// NextItem is `admin.item.next {}` (D-65, which also IS ADMN-02's item.skip): in
// ITEM_INTRO it marks the current item skipped, in SETTLED it moves on, and with
// no pending item left it enters FINALE with no current item.
func (e *Engine) NextItem() error {
	return fmt.Errorf("game: NextItem: %w", errREDPhase3Items)
}
