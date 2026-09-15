package game

import (
	"errors"
	"fmt"
)

// This file is the English auction engine: the live run (D-49), the ordered bid
// ladder (D-52/D-53), the anti-snipe extension (D-59) and the one settle path
// shared by the deadline, the admin's force-end and the boot (D-55).

// errREDPhase3Auction is the RED-phase scaffold of plan 03-01's second task: the
// auction mutations refuse to run until the GREEN commit implements them. It
// exists only so the failing tests compile as tests (a load failure would be
// INVALID_RED, #3770).
var errREDPhase3Auction = errors.New("game: the auction engine is not implemented yet")

// The four reasons a bid can be refused, in the D-52 order. These are
// payload-level strings (D-53): the frozen error catalog keeps its seven values,
// and the client maps each reason to its own copy.
const (
	ReasonInvalidAmount  = "invalid_amount"
	ReasonWindowClosed   = "window_closed"
	ReasonBelowMinimum   = "below_minimum"
	ReasonExceedsBudget  = "exceeds_budget"
	ReasonAlreadyLeading = "already_leading"
)

// ErrNotLive reports that no live auction exists to settle — the phase edge
// LIVE → SETTLED is the only one a settle may take (D-55/D-65).
var ErrNotLive = errors.New("game: no live auction to settle")

// BidRejectedError is what PlaceBid returns after the ladder refused a bid. The
// rejection itself already travelled to the submitter through the emit callback
// (D-53); the error exists so the caller can tell a rejection (nothing
// persisted, nothing published) apart from a real engine failure.
type BidRejectedError struct{ Reason string }

func (e *BidRejectedError) Error() string { return "game: bid rejected: " + e.Reason }

// auctionRun is the in-memory live state of one item's run (D-49): the timing
// that the `auction_runtime` row persists, plus the settle reveal. The current
// price and top bidder are NOT fields — they are derived from the bid trail
// below, which is the same rule boot reconstruction uses (H4/H5), so a replayed
// item can never resurrect a previous run's price.
type auctionRun struct {
	itemID         string
	startedAt      int64
	deadlineAt     int64 // 0 for an item type with no deadline
	extensionsUsed int
	settledAt      int64
	settle         *SettleInfo
}

// bid is one accepted bid in the process-wide append-only trail (H4/D-51). A
// bid belongs to a run when its itemID matches and its createdAt is at or after
// the run's startedAt.
type bid struct {
	itemID    string
	playerID  string
	amount    int64
	createdAt int64
}

// PlaceBid runs the D-52 ladder in order and, on success, commits one mutation
// (rev++, the bid row, the runtime row when the extension moved the deadline)
// and fans out `bid.accepted`. On refusal it calls emit ONCE, inside the same
// critical section (the Join precedent), and returns a *BidRejectedError —
// nothing persisted, nothing published, no revision change (D-53).
func (e *Engine) PlaceBid(playerID string, amount int64, emit func(View, BidRejection)) error {
	return fmt.Errorf("game: PlaceBid: %w", errREDPhase3Auction)
}

// SettleIfDue settles a LIVE auction whose wall-clock deadline has passed. It is
// the ticker's trigger (D-55 a/D-56) and re-checks the phase and the deadline
// under the lock, so a force-end that lands first makes it a clean no-op. The
// bool reports whether this call settled.
func (e *Engine) SettleIfDue() (bool, error) {
	return false, fmt.Errorf("game: SettleIfDue: %w", errREDPhase3Auction)
}

// ForceEnd settles a LIVE auction immediately (D-55 b / D-65). Outside LIVE it
// refuses with ErrNotLive and changes nothing.
func (e *Engine) ForceEnd() error {
	return fmt.Errorf("game: ForceEnd: %w", errREDPhase3Auction)
}
