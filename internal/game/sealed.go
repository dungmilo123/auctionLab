package game

// This file is the sealed-bid auction engine (Phase 6): the ONE submit path
// (D-108/D-109/D-110/D-111) and — from the settle plan of this phase — the ONE
// winner/charge derivation every surface reads (D-113/D-114).
//
// Nothing here is a second settle path and nothing here persists a price: a
// submit writes ONE effective row per (item, player) plus ONE audit row in the
// SAME mutation, and the private confirmation carries a count and nothing else
// (D-111).
//
// RED (plan 06-01, task 1): the surface below is declared so the engine, store
// and hub suites compile and fail on their assertions. The mutation itself is
// the GREEN step of this task.

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
// RED: not implemented yet.
func (e *Engine) SubmitSealed(playerID string, amount int64, emit func(View, BidRejection)) error {
	return nil
}
