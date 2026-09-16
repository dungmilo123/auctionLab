package game

import "fmt"

// This file is the Dutch auction engine (Phase 5): the ONE pure price
// derivation (D-91/DTCH-01), the serialized accept that settles the item at the
// server's own number (D-95/D-96) and the type-aware winner lookup the shared
// settle path reads (D-98/D-100).
//
// Nothing here is a second settle path and nothing here persists a price. The
// accept records ONE entry in the append-only trail — carrying the `accept`
// kind, so no English ladder read can ever see it — and then calls the existing
// settleLocked, which keeps the budget arithmetic and the one reveal
// constructor in their single home (D-98/D-100).

// ReasonAlreadySold is D-97's payload-level reason: the item the player just
// tried to accept was SOLD by another player's acceptance. It is a payload-level
// string like the ladder's five reasons (D-53) — the frozen error catalog keeps
// its seven values — and the client maps it to its own copy.
const ReasonAlreadySold = "already_sold"

// dutchPriceAt is D-91's ONE price derivation, and the only place a Dutch price
// is ever computed: the projection, the tick frame, the accept validation and
// the settle all call it at their own instant, and no price is persisted per
// tick (DTCH-01).
//
// It is total: a nil item answers 0, a missing or non-sensical mechanic answers
// the start price, and an elapsed in the past clamps to zero, so a price above
// start_price is never returned.
func dutchPriceAt(it *item, startedAt, now int64) int64 {
	if it == nil {
		return 0
	}
	if it.decrementStep < 1 || it.decrementIntervalMS < 1 {
		// A row without usable mechanics has no clock: the start price is the
		// only honest number (D-92 refuses to start such a run at all).
		return it.startPrice
	}

	elapsed := now - startedAt
	if elapsed < 0 {
		elapsed = 0 // a skewed clock never buys a price above the start (D-91)
	}
	steps := elapsed / it.decrementIntervalMS

	// Clamp the step count in the DIVISION domain so `steps × step` cannot
	// overflow int64. Past the floor the price is pinned anyway, so a larger
	// count is indistinguishable — while a wrapped product lands somewhere
	// between the floor and the start, i.e. a plausible-looking but WRONG price
	// that no later clamp catches (research Pitfall 4). A floor at or above the
	// start never steps down at all.
	if span := it.startPrice - it.floorPrice; span >= 0 {
		if limit := span/it.decrementStep + 1; steps > limit {
			steps = limit
		}
	} else {
		steps = 0
	}

	price := it.startPrice - steps*it.decrementStep
	if price < it.floorPrice {
		price = it.floorPrice
	}
	return price
}

// addDutchLiveFields is D-94's Dutch half of the public live fields: the price
// derived at the projection's own instant, the anchor it was derived from, and
// the three mechanics a client needs to re-derive it between frames. It lives
// here rather than in project.go so every Dutch number is computed in one file,
// beside the ONE derivation.
//
// Once the run holds an acceptance the CHARGED price is the number that was
// real: a settled Dutch run re-derives past the floor, and reporting that would
// make the item disagree with the reveal attached to it (D-98).
func (s *state) addDutchLiveFields(view *PublicItem, it *item, run *auctionRun, now int64) {
	if w, ok := s.runAcceptance(); ok {
		price := w.amount
		view.CurrentPrice = &price
	} else {
		price := dutchPriceAt(it, run.startedAt, now)
		view.CurrentPrice = &price
	}

	startedAt := run.startedAt
	view.StartedAt = &startedAt

	// The project's `> 0` convention for an optional number (see adminItem):
	// D-92 refuses to start a Dutch run without these mechanics, so a live run
	// always carries all three, and a client that sees one missing knows the row
	// is not playable rather than that its price is zero.
	if it.floorPrice > 0 {
		v := it.floorPrice
		view.FloorPrice = &v
	}
	if it.decrementStep > 0 {
		v := it.decrementStep
		view.DecrementStep = &v
	}
	if it.decrementIntervalMS > 0 {
		v := it.decrementIntervalMS
		view.DecrementIntervalMS = &v
	}
}

// runAcceptance returns the acceptance recorded for the current run: the LAST
// trail entry of kind `accept`, scoped to the run exactly the way topBid scopes
// a ladder bid (itemID + createdAt >= startedAt). It is how a settled Dutch run
// finds its winner — including after a restart, which is why the row is durable
// — and it is the read that keeps an accept out of the English ladder (D-98).
func (s *state) runAcceptance() (bid, bool) {
	run := s.currentAuction
	if run == nil {
		return bid{}, false
	}
	for i := len(s.bids) - 1; i >= 0; i-- {
		b := s.bids[i]
		if b.kind == trailAccept && b.itemID == run.itemID && b.createdAt >= run.startedAt {
			return b, true
		}
	}
	return bid{}, false
}

// runWinner is the ONE type-aware winner lookup (D-100): the English ladder's
// top bid, or the Dutch run's recorded acceptance. settleLocked and settleReveal
// both read it, so a Dutch settle can grow neither a second settle path nor a
// second reveal constructor — and a force-ended Dutch run with no acceptance is
// `unsold` through exactly the same branch an English run with no bids takes
// (D-102).
func (s *state) runWinner() (bid, bool) {
	if it := s.items[s.currentItemID]; it != nil && it.auctionType == "dutch" {
		return s.runAcceptance()
	}
	return s.topBid()
}

// AcceptDutch is `dutch.accept {}` (D-95/D-96): the ordered validation, then ONE
// mutation that records the acceptance and settles the item at the price the
// server derives for this instant.
//
// The ORDER is the contract (D-96/D-97): an unknown player is a programmer
// error, a run sold by ANOTHER acceptance is already_sold, every other closed
// window is window_closed, and a price above the budget is exceeds_budget —
// the same "the order is the contract" discipline the D-52 ladder keeps.
//
// A refusal is NOT a mutation: it calls emit once, inside this same critical
// section (the PlaceBid precedent), and returns a *BidRejectedError — nothing
// persisted, nothing published, no revision change (D-53/D-96).
func (e *Engine) AcceptDutch(playerID string, emit func(View, BidRejection)) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	p := e.state.players[playerID]
	if p == nil {
		// Not a user error: the hub only routes accepts from registered players.
		return fmt.Errorf("game: AcceptDutch for unknown player %q", playerID)
	}

	now := e.now().UnixMilli()
	it := e.state.items[e.state.currentItemID]
	run := e.state.currentAuction

	// ONE derivation at the accept instant: the number a refusal reports is the
	// number a successful accept charges (DTCH-01/DTCH-02).
	var startedAt int64
	if run != nil {
		startedAt = run.startedAt
	}
	price := dutchPriceAt(it, startedAt, now)

	reject := func(reason string) error {
		// D-96: min_next_bid stays null — a Dutch price has no ladder — and the
		// amount is the price at THIS instant, so the sentence the player reads
		// names a real number.
		rejection := BidRejection{Reason: reason, Budget: p.budget, Amount: price}
		if emit != nil {
			emit(view{e}, rejection)
		}
		return &BidRejectedError{Reason: reason}
	}

	// 2a (D-97): this run was sold by ANOTHER player's acceptance of this very
	// Dutch item. It must be answered BEFORE the generic not-LIVE check: the
	// winner's accept settled inside this same lock, so an accept that arrives
	// second sees a SETTLED phase — and window_closed would tell the loser that
	// the window simply ran out instead of that somebody accepted first.
	if it != nil && it.auctionType == "dutch" && run != nil && run.itemID == it.id &&
		run.settle != nil && run.settle.Outcome == "sold" {
		return reject(ReasonAlreadySold)
	}
	// 2b (D-96): every other closed window — not LIVE, not a Dutch item, a run
	// the presenter already advanced past, a run that settled without an
	// acceptance (a force-end is `unsold`, not `already_sold`).
	if it == nil || run == nil || run.itemID != it.id ||
		e.state.phase != PhaseLive || it.auctionType != "dutch" {
		return reject(ReasonWindowClosed)
	}
	// 3: the SERVER's price at this instant is the only amount that exists.
	if price > p.budget {
		return reject(ReasonExceedsBudget)
	}

	// 4: the win — the acceptance, the settle and their audit rows in ONE tx.
	return e.applyLocked(func(s *state) error {
		return s.acceptDutchLocked(p.id, price, now)
	}, true, nil)
}

// acceptDutchLocked is the in-memory half of the accept: record the acceptance
// in the ONE append-only trail, then let the EXISTING settle path produce the
// whole fan-out. It deliberately appends no frame of its own — settleLocked
// appends item.settled → leaderboard → state.snapshot, and the trailing snapshot
// must stay LAST because it is what a client that missed everything else resyncs
// from (D-41).
func (s *state) acceptDutchLocked(playerID string, price, now int64) error {
	run := s.currentAuction
	s.bids = append(s.bids, bid{
		itemID: run.itemID, playerID: playerID,
		amount: price, createdAt: now, kind: trailAccept,
	})
	// The caller validated LIVE and the run consistency under this same lock —
	// exactly the guard settleLocked re-checks — so this cannot fail; returning
	// its error is what keeps a failed mutation from being committed.
	return s.settleLocked(now)
}
