package game

import (
	"errors"
	"fmt"
)

// This file is the English auction engine: the live run (D-49), the ordered bid
// ladder (D-52/D-53), the anti-snipe extension (D-59) and the one settle path
// shared by the deadline, the admin's force-end and the boot (D-55).

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

// The trail kinds the append-only `bids` audit carries (PLAN.md §5's vocabulary
// `bid | accept | sealed_edit`). The kind is what keeps the two flows apart in
// ONE audit trail: an English ladder read only ever sees `bid`, and a Dutch
// acceptance is only ever read as `accept` — so an accept can neither join the
// ladder nor light the English outbid banner (D-98), and a future `sealed_edit`
// row is excluded from both by construction.
const (
	trailBid    = "bid"
	trailAccept = "accept"
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
// the run's startedAt, and `kind` says which flow wrote it: `bid` for the
// English ladder, `accept` for a Dutch acceptance (D-98).
type bid struct {
	itemID    string
	playerID  string
	amount    int64
	createdAt int64
	kind      string
}

// currentRun returns the open auction run, or nil when no item is running.
func (s *state) currentRun() *auctionRun { return s.currentAuction }

// topBid returns the accepted bid that currently leads the run: the last bid in
// the append-only trail that belongs to the current item and run (H4/H5). The
// run boundary is startedAt, which is the same rule boot reconstruction uses,
// so a replayed item cannot resurrect a previous run's price.
//
// The `bid` kind filter is D-98: a Dutch acceptance rides the same trail, and
// the English ladder must never read one as a bid — so this is the ONE place
// that decides what "a bid" means, and currentPrice/minNextBid inherit it.
func (s *state) topBid() (bid, bool) {
	run := s.currentAuction
	if run == nil {
		return bid{}, false
	}
	for i := len(s.bids) - 1; i >= 0; i-- {
		b := s.bids[i]
		if b.kind == trailBid && b.itemID == run.itemID && b.createdAt >= run.startedAt {
			return b, true
		}
	}
	return bid{}, false
}

// lastBidOf returns one player's most recent accepted bid in the current run —
// the value behind PlayerMe.last_bid (D-67). It is scoped to the run on purpose:
// a bid from the previous item must not light the outbid banner (D-72), and a
// Dutch acceptance is not one of this player's bids (D-98).
func (s *state) lastBidOf(playerID string) (bid, bool) {
	run := s.currentAuction
	if run == nil {
		return bid{}, false
	}
	for i := len(s.bids) - 1; i >= 0; i-- {
		b := s.bids[i]
		if b.kind == trailBid && b.playerID == playerID && b.itemID == run.itemID && b.createdAt >= run.startedAt {
			return b, true
		}
	}
	return bid{}, false
}

// currentPrice is the top bid, or the item's starting price before the first
// bid: the one number every projection and the ladder share (D-54).
func (s *state) currentPrice() (int64, bool) {
	run := s.currentAuction
	if run == nil {
		return 0, false
	}
	if top, ok := s.topBid(); ok {
		return top.amount, true
	}
	it := s.items[run.itemID]
	if it == nil {
		return 0, false
	}
	return it.startPrice, true
}

// minNextBid is the smallest amount the ladder will accept right now: the
// starting price before the first bid, current + increment after it. The server
// computes it and every live payload carries it (D-54) — clients never do this
// arithmetic (D-71).
func (s *state) minNextBid() (int64, bool) {
	run := s.currentAuction
	if run == nil {
		return 0, false
	}
	it := s.items[run.itemID]
	if it == nil {
		return 0, false
	}
	if top, ok := s.topBid(); ok {
		return top.amount + it.minIncrement, true
	}
	return it.startPrice, true
}

// topBidderNickname resolves the leader's public identity, or ("", false).
func (s *state) topBidderNickname() (string, bool) {
	top, ok := s.topBid()
	if !ok {
		return "", false
	}
	p := s.players[top.playerID]
	if p == nil {
		return "", false
	}
	return p.nickname, true
}

// acceptBid applies one accepted bid to the scratch state. It runs only after
// the ladder cleared the amount, so it cannot fail:
//
//   - the trail append (the in-memory half of the append-only `bids` audit);
//   - the D-59 anti-snipe extension, inside the final window and under the cap,
//     moving the deadline to now + window — never deadline + window;
//   - the D-54 `bid.accepted` event, built from the numbers the committed state
//     will carry, so it needs no second lock section.
func (s *state) acceptBid(playerID string, amount, now int64, it *item) {
	run := s.currentAuction
	s.bids = append(s.bids, bid{itemID: run.itemID, playerID: playerID, amount: amount, createdAt: now, kind: trailBid})

	if run.deadlineAt > 0 && it.antiSnipeMode == "extend" && it.antiSnipeWindowMS > 0 {
		remaining := run.deadlineAt - now
		underCap := it.maxExtensions <= 0 || int64(run.extensionsUsed) < it.maxExtensions
		if remaining <= it.antiSnipeWindowMS && underCap {
			run.deadlineAt = now + it.antiSnipeWindowMS
			run.extensionsUsed++
		}
	}

	payload := BidAccepted{
		PlayerID:       playerID,
		Amount:         amount,
		CurrentPrice:   amount,
		MinNextBid:     amount + it.minIncrement,
		ExtensionsUsed: run.extensionsUsed,
	}
	if p := s.players[playerID]; p != nil {
		payload.Nickname = p.nickname
	}
	if run.deadlineAt > 0 {
		deadline := run.deadlineAt
		payload.DeadlineAt = &deadline
	}
	s.events = append(s.events, Event{Type: EventBidAccepted, Data: payload})
}

// settleLocked is the ONLY place budgets move (D-55). It refuses when the game
// is not LIVE, which is exactly what makes a force-end and a due tick settle
// once no matter which of them lands first (the double-pay guard, T-03-02).
func (s *state) settleLocked(now int64) error {
	if s.phase != PhaseLive || s.currentAuction == nil {
		return ErrNotLive
	}
	run := s.currentAuction
	it := s.items[run.itemID]
	if it == nil {
		return fmt.Errorf("game: settle for unknown item %q", run.itemID)
	}

	// D-100: the winner lookup is type-aware — the English ladder's top bid, or
	// the Dutch run's recorded acceptance. One accessor, so a Dutch settle takes
	// exactly this branch rather than growing a settle of its own.
	top, hasWinner := s.runWinner()
	if hasWinner {
		p := s.players[top.playerID]
		if p == nil {
			return fmt.Errorf("game: settle for unknown player %q", top.playerID)
		}
		// ECON-02: the winner pays their own bid exactly once, and the item's
		// truth value moves into total_value. price <= budget was enforced at
		// bid time and budgets only move here, so the result cannot go negative.
		p.budget = p.budget - top.amount + it.truthValue
		p.totalValue += it.truthValue
	}

	// The reveal is built by the SAME helper the projection uses, so a process
	// that restarts on this settled file derives an object identical to the one
	// stored here (D-62/D-67).
	reveal := s.settleReveal(it)

	it.status = "settled"
	run.settledAt = now
	run.settle = reveal
	s.phase = PhaseSettled

	// D-55/D-57/D-58: one settle, one revision, three sibling frames — the
	// reveal, the updated standings, and the authoritative snapshot (which is
	// what a client that missed a frame resyncs from).
	s.events = append(s.events,
		Event{Type: EventItemSettled, Data: s.currentPublicItem(now)},
		Event{Type: EventLeaderboard, Data: s.leaderboard()},
		Event{Type: EventSnapshot},
	)
	return nil
}

// PlaceBid runs the D-52 ladder in order and, on success, commits one mutation
// (rev++, the bid row, the runtime row when the extension moved the deadline)
// and records the `bid.accepted` fan-out. On refusal it calls emit ONCE, inside
// the same critical section (the Join precedent), and returns a
// *BidRejectedError — nothing persisted, nothing published, no revision change
// (D-53).
func (e *Engine) PlaceBid(playerID string, amount int64, emit func(View, BidRejection)) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	p := e.state.players[playerID]
	if p == nil {
		// Not a user error: the hub only routes bids from registered players.
		return fmt.Errorf("game: PlaceBid for unknown player %q", playerID)
	}

	now := e.now().UnixMilli()
	it := e.state.items[e.state.currentItemID]
	run := e.state.currentAuction

	reject := func(reason string) error {
		rejection := BidRejection{Reason: reason, Budget: p.budget, Amount: amount}
		if min, ok := e.state.minNextBid(); ok {
			rejection.MinNextBid = &min
		}
		if emit != nil {
			emit(view{e}, rejection)
		}
		return &BidRejectedError{Reason: reason}
	}

	// The ladder, in this exact order — the ORDER is the contract (D-52): a
	// late and too-low bid must read window_closed, and a leading bidder's
	// over-budget bid must still read exceeds_budget.
	if amount <= 0 {
		return reject(ReasonInvalidAmount)
	}
	if it == nil || run == nil || run.itemID != it.id || e.state.phase != PhaseLive || now >= run.deadlineAt {
		// The closed window is checked against wall-clock now, so a bid that
		// arrives after the deadline is refused even before the ticker settles
		// (research Q7: the clock decides, not the ticker).
		return reject(ReasonWindowClosed)
	}
	if min, ok := e.state.minNextBid(); ok && amount < min {
		return reject(ReasonBelowMinimum)
	}
	if amount > p.budget {
		return reject(ReasonExceedsBudget)
	}
	if top, ok := e.state.topBid(); ok && top.playerID == playerID {
		return reject(ReasonAlreadyLeading)
	}

	return e.applyLocked(func(s *state) error {
		s.acceptBid(playerID, amount, now, s.items[it.id])
		return nil
	}, true, nil)
}

// SettleIfDue settles a LIVE auction whose wall-clock deadline has passed. It is
// the ticker's trigger (D-55 a / D-56) and re-checks the phase and the deadline
// under the lock, so a force-end that lands first makes it a clean no-op. The
// bool reports whether this call settled.
func (e *Engine) SettleIfDue() (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	run := e.state.currentAuction
	if e.state.phase != PhaseLive || run == nil || run.deadlineAt == 0 {
		return false, nil
	}
	now := e.now().UnixMilli()
	if now < run.deadlineAt {
		return false, nil
	}
	if err := e.applyLocked(func(s *state) error { return s.settleLocked(now) }, true, nil); err != nil {
		return false, err
	}
	return true, nil
}

// ForceEnd settles a LIVE auction immediately (D-55 b / D-65). Outside LIVE it
// refuses with ErrNotLive and changes nothing — the controller maps that to a
// private `bad_message` (H12).
func (e *Engine) ForceEnd() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state.phase != PhaseLive || e.state.currentAuction == nil {
		return ErrNotLive
	}
	now := e.now().UnixMilli()
	return e.applyLocked(func(s *state) error { return s.settleLocked(now) }, true, nil)
}
