package game

// This file is the Dutch auction engine (Phase 5): the ONE pure price
// derivation (D-91/DTCH-01) and the serialized accept that settles the item at
// the server's own number (D-95…D-98).
//
// RED (plan 05-01, task 1): the API surface below is declared so the derivation
// table and the accept tests compile and fail on their assertions. The
// derivation itself, the accept mutation and the settle wiring are the GREEN
// step of this task.

// dutchPriceAt is D-91's ONE price derivation ...
//
// RED: not implemented yet.
func dutchPriceAt(it *item, startedAt, now int64) int64 {
	return 0
}

// AcceptDutch is the `dutch.accept` engine entry point (D-95/D-96) ...
//
// RED: not implemented yet.
func (e *Engine) AcceptDutch(playerID string, emit func(View, BidRejection)) error {
	return nil
}
