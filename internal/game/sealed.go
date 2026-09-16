package game

import (
	"fmt"
	"slices"
)

// This file is the sealed-bid auction engine (Phase 6): the ONE submit path
// (D-108/D-109/D-110/D-111) and — from the settle plan of this phase — the ONE
// winner/charge derivation every surface reads (D-113/D-114).
//
// Nothing here is a second settle path and nothing here persists a price: a
// submit writes ONE effective row per (item, player) plus ONE audit row in the
// SAME mutation, and the private confirmation carries a count and nothing else
// (D-111).

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
