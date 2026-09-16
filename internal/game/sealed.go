package game

import (
	"fmt"
	"slices"
	"sort"
)

// This file is the sealed-bid auction engine (Phase 6): the ONE submit path
// (D-108/D-109/D-110/D-111) and the ONE winner/charge derivation every surface
// reads (D-113/D-114).
//
// Nothing here is a second settle path and nothing here persists a price: a
// submit writes ONE effective row per (item, player) plus ONE audit row in the
// SAME mutation, and the private confirmation carries a count and nothing else
// (D-111). The settle reads sealedOutcome, which the existing settle path and
// the boot's rebuild both call.

// trailSealedEdit is the third kind of the append-only `bids` audit (PLAN.md §5's
// vocabulary `bid | accept | sealed_edit`, D-109): one row per sealed submit or
// edit, written in the SAME transaction as the effective row and with the
// instant of that request. It is audit-only — every ladder and feed reader
// filters on `kind`, so a sealed edit can never join the English ladder (D-98).
const trailSealedEdit = "sealed_edit"

// SubmitSealed is the `sealed.submit {amount}` engine entry point (D-111). It
// follows the PlaceBid template — lock, then validate, then ONE applyLocked
// mutation and the fan-out inside the same critical section — and answers a
// refusal with exactly one private callback and no mutation (D-53's shape).
//
// The command is not a mutation of the ladder: an edit UPSERTS the player's
// effective row (D-108), appends ONE audit row (D-109) and answers the
// submitter alone (D-111). The amount is the only client-supplied value; the
// player id comes from the connection, so nothing in the body can name another
// player.
func (e *Engine) SubmitSealed(playerID string, amount int64, emit func(View, BidRejection)) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	p := e.state.players[playerID]
	if p == nil {
		// Not a user error: the hub only routes submits from registered players.
		return fmt.Errorf("game: SubmitSealed for unknown player %q", playerID)
	}

	now := e.now().UnixMilli()
	it := e.state.items[e.state.currentItemID]
	run := e.state.currentAuction

	reject := func(reason string) error {
		// D-53's shape: ONE private callback inside this same critical section —
		// the budget and the amount it reports are the numbers of this moment —
		// and nothing persisted, published or revised.
		rejection := BidRejection{Reason: reason, Budget: p.budget, Amount: amount}
		// D-112 #4/SEAL-02: below_minimum reports the ITEM's own floor —
		// start_price — and never the English ladder's next bid, which is right
		// for a sealed run only by accident and absent for a Dutch refusal that
		// deliberately carries none.
		if reason == ReasonBelowMinimum && it != nil {
			min := it.startPrice
			rejection.MinNextBid = &min
		}
		if emit != nil {
			emit(view{e}, rejection)
		}
		return &BidRejectedError{Reason: reason}
	}

	if it == nil || run == nil || run.itemID != it.id || e.state.phase != PhaseLive ||
		!slices.Contains(sealedTypes, it.auctionType) || now >= run.deadlineAt {
		// The window is checked FIRST and by TYPE (D-93's discipline): a sealed
		// submission needs a live SEALED run whose deadline is still in the
		// future. `now >= run.deadlineAt` is the wall clock's own verdict, so a
		// submit that arrives after close is refused even before the ticker
		// settles the item — and a run whose deadline is 0 (a row the type is
		// not allowed to have) is closed by the same comparison rather than by a
		// branch of its own.
		return reject(ReasonWindowClosed)
	}

	// The amount ladder, in this exact order — the ORDER is the contract
	// (D-112), and it is deliberately NOT PlaceBid's (which checks the amount
	// first): a closed window must read window_closed even when the amount was
	// also impossible, and a bid that is both under the floor and over the
	// budget must read below_minimum. Both bounds are INCLUSIVE: `[start_price,
	// budget]` are the amounts SEAL-02 permits.
	if amount <= 0 {
		return reject(ReasonInvalidAmount)
	}
	if amount < it.startPrice {
		return reject(ReasonBelowMinimum)
	}
	if amount > p.budget {
		return reject(ReasonExceedsBudget)
	}

	return e.applyLocked(func(s *state) error {
		s.applySealedSubmit(p.id, amount, now)
		return nil
	}, true, nil)
}

// sealedBidOf returns the effective row of one (item, player), or false. It is
// the ONE lookup the submit upsert and the projections read (D-108/D-115), and
// it hands back a pointer into the slice so an edit rewrites the row in place
// instead of appending a second one.
func (s *state) sealedBidOf(itemID, playerID string) (*sealedBid, bool) {
	for i := range s.sealedBids {
		if s.sealedBids[i].itemID == itemID && s.sealedBids[i].playerID == playerID {
			return &s.sealedBids[i], true
		}
	}
	return nil, false
}

// sealedOutcome is the sealed run's WHOLE settlement — the winner, the price
// the settle charges and whether there is a winner at all — derived from the
// effective set alone (D-113/D-114). It is one pure function in the shape
// dutchPriceAt set: the one settle path and the one reveal constructor (and
// therefore the boot's rebuild) all read it, so the price the projector
// publishes and the price the winner pays can never disagree.
//
// It is total: a missing item, no rows, or rows that all belong to another item
// answer `unsold`, and the one-bidder Vickrey case falls out of
// max(0, start_price) rather than out of a branch of its own (SEAL-04).
func sealedOutcome(it *item, sheets []sealedBid) (winner sealedBid, price int64, ok bool) {
	if it == nil {
		return sealedBid{}, 0, false
	}

	// The set is the CURRENT item's rows only: s.sealedBids accumulates across
	// items until a reset clears it, so an unscoped scan would settle today's
	// item on the previous item's sheet (research Pitfall 10).
	set := make([]sealedBid, 0, len(sheets))
	for _, sheet := range sheets {
		if sheet.itemID == it.id {
			set = append(set, sheet)
		}
	}
	if len(set) == 0 {
		return sealedBid{}, 0, false
	}

	// The TOTAL order (D-113/SEAL-05): the highest effective amount first, the
	// earliest FIRST submission next, and the player id last. The final rung
	// exists only so a pair that ties on both real rules is still decidable —
	// the same documented last rung ECON-05's standing keeps — which is also
	// why sorting can never fall back on the order the rows happen to sit in.
	sort.SliceStable(set, func(i, j int) bool {
		a, b := set[i], set[j]
		switch {
		case a.amount != b.amount:
			return a.amount > b.amount
		case a.submittedAt != b.submittedAt:
			return a.submittedAt < b.submittedAt
		default:
			return a.playerID < b.playerID
		}
	})
	winner = set[0]

	switch it.auctionType {
	case "vickrey":
		// SEAL-04: the runner-up's effective amount, floored at the item's
		// start price. One bidder leaves `second` at zero, so the floor IS the
		// one-bidder rule; a top-two tie charges the tied amount, which can
		// never exceed the winner's own bid because the submit path enforced
		// amount >= start_price.
		var second int64
		if len(set) > 1 {
			second = set[1].amount
		}
		price = second
		if price < it.startPrice {
			price = it.startPrice
		}
	default: // sealed_first
		price = winner.amount
	}
	return winner, price, true
}

// applySealedSubmit applies ONE submission to the scratch state. It runs only
// after the window guard cleared the request, so it cannot fail:
//
//   - the effective row: created on the first submit, UPDATED (amount and
//     updated_at only) on every edit, so `submitted_at` — the SEAL-05 tie-break
//     instant — never moves and the count never grows (D-108);
//   - ONE audit row in the append-only `bids` trail, in the same mutation, with
//     its own `sealed_edit` kind (D-109);
//   - the private confirmation, then the trailing snapshot (D-41/D-111).
func (s *state) applySealedSubmit(playerID string, amount, now int64) {
	it := s.items[s.currentItemID]
	if row, ok := s.sealedBidOf(it.id, playerID); ok {
		row.amount = amount
		row.updatedAt = now
		// submitted_at is deliberately NOT touched here — and not in the store's
		// DO UPDATE SET either. That omission IS D-108/SEAL-05.
	} else {
		s.sealedBids = append(s.sealedBids, sealedBid{
			itemID: it.id, playerID: playerID,
			amount: amount, submittedAt: now, updatedAt: now,
		})
	}

	s.bids = append(s.bids, bid{
		itemID: it.id, playerID: playerID,
		amount: amount, createdAt: now, kind: trailSealedEdit,
	})

	// D-111: the count the submitter's confirmation carries is the SAME number
	// the projector renders, from the ONE counting rule (project.go's
	// sealedProgress). The fallback is unreachable — the window guard above just
	// proved the current item is sealed — and exists so a projection helper can
	// never nil-panic inside the engine lock.
	progress := SealedProgress{Total: len(s.players)}
	if counted := s.sealedProgress(); counted != nil {
		progress = *counted
	}
	s.events = append(s.events,
		Event{Type: EventSealedReceived, To: playerID, Data: progress},
		Event{Type: EventSnapshot}, // LAST: the resync anchor (D-41)
	)
}
