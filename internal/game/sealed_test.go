package game

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"auctionlab/internal/store"
)

// This file is SEAL-01/SEAL-02/SEAL-05's engine contract for the sealed flow
// (D-108…D-112): ONE effective row per (item, player) that an edit UPSERTS —
// the first submission's instant frozen because SEAL-05's tie-break reads it —
// plus one audit-only `sealed_edit` row per request, in the same mutation.
//
// Time is data here (D-38): every case moves env.clock instead of sleeping, so
// a sealed window is verified in microseconds and the expected instants are
// exact literals.

// sealedFixture is one complete sealed-first-price item: a wall-clock window
// (D-107) with anti-snipe disabled, because a sealed window is never extended.
func sealedFixture() *item {
	return &item{
		id: "it-s1", orderIndex: 0, name: "Tranh", description: "Sealed lot",
		auctionType: "sealed_first", truthValue: fixtureTruthValue,
		hintMin: 200_000, hintMax: 500_000, startPrice: 100_000,
		durationMS: 30_000, antiSnipeMode: "disabled",
		status: "live",
	}
}

// mustOpenSealedRun installs the item and opens a run on it. Like
// mustOpenLiveRun it seeds the item row through the store write set first (the
// row is a real foreign-key target) and opens the run through Apply — the one
// production mutation path — with `deadline_at = now + duration_ms` exactly as
// English does (D-107).
func mustOpenSealedRun(t *testing.T, env *testEnv, it *item) {
	t.Helper()

	now := env.clock.now().UnixMilli()
	if err := env.store.Commit(store.Mutation{
		Phase:     string(env.engine.Phase()),
		Revision:  env.engine.Rev(),
		UpdatedAt: now,
		Items:     []store.ItemRow{itemRow(it)},
	}); err != nil {
		t.Fatalf("seeding the sealed item row: %v", err)
	}

	if err := env.engine.Apply(func(s *state) error {
		s.items[it.id] = it
		s.itemOrder = append(s.itemOrder, it.id)
		s.currentItemID = it.id
		s.phase = PhaseLive
		s.currentAuction = &auctionRun{itemID: it.id, startedAt: now, deadlineAt: now + it.durationMS}
		return nil
	}); err != nil {
		t.Fatalf("opening a live sealed run: %v", err)
	}
}

// submitSealed calls SubmitSealed and captures the private refusal, if any —
// the sealed twin of placeBid (D-53/D-111).
func (env *testEnv) submitSealed(playerID string, amount int64) (BidRejection, bool, error) {
	var (
		rejection BidRejection
		got       bool
	)
	err := env.engine.SubmitSealed(playerID, amount, func(_ View, r BidRejection) {
		rejection, got = r, true
	})
	return rejection, got, err
}

// sealedRefusal submits an amount the engine is expected to refuse and returns
// the private refusal. A refusal IS a *BidRejectedError (D-53), so the error is
// part of the contract rather than a failure: what must hold is that the caller
// was answered privately AND that the reason is the one the order demands.
func (env *testEnv) sealedRefusal(t *testing.T, playerID string, amount int64) BidRejection {
	t.Helper()

	rejection, refused, err := env.submitSealed(playerID, amount)
	if !refused {
		t.Fatalf("SubmitSealed(%d) was not answered privately (err = %v)", amount, err)
	}
	var rejectedErr *BidRejectedError
	if !errors.As(err, &rejectedErr) {
		t.Fatalf("SubmitSealed(%d) error = %v, want a *BidRejectedError", amount, err)
	}
	return rejection
}

// mustSubmitSealed submits an amount the engine must accept, failing the case
// with the refusal it got instead.
func (env *testEnv) mustSubmitSealed(t *testing.T, playerID string, amount int64) {
	t.Helper()

	if rejection, refused, err := env.submitSealed(playerID, amount); err != nil || refused {
		t.Fatalf("SubmitSealed(%d) was refused: err = %v, refusal = %+v", amount, err, rejection)
	}
}

// sealedRowOf reads the ONE effective row of one (item, player) out of the
// engine's own state, under the lock, so the assertion is about the engine's
// numbers rather than a projection copy.
func (env *testEnv) sealedRowOf(t *testing.T, itemID, playerID string) (sealedBid, bool) {
	t.Helper()

	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	found := 0
	var row sealedBid
	for _, b := range env.engine.state.sealedBids {
		if b.itemID == itemID && b.playerID == playerID {
			row, found = b, found+1
		}
	}
	if found > 1 {
		t.Errorf("the engine holds %d effective rows for (%s, %s), want at most 1 (D-108)", found, itemID, playerID)
	}
	return row, found == 1
}

// sealedConfirmationOf returns the private `sealed.received` payload the most
// recent mutation emitted for one player (D-111). Its TYPE is the contract: a
// SealedProgress cannot carry an amount (D-88/SEAL-01).
func (env *testEnv) sealedConfirmationOf(t *testing.T, playerID string) SealedProgress {
	t.Helper()

	for _, event := range env.lastEvents() {
		if event.Type == EventSealedReceived && event.To == playerID {
			progress, ok := event.Data.(SealedProgress)
			if !ok {
				t.Fatalf("sealed.received payload = %T, want the count-only SealedProgress (D-88)", event.Data)
			}
			return progress
		}
	}
	t.Fatalf("no private sealed.received event for %q: %+v", playerID, env.lastEvents())
	return SealedProgress{}
}

// TestSubmitSealedRejectOrder is D-112's contract: the ORDER of the refusals is
// the contract (mirroring D-52), the window is decided before any amount is
// examined, and the amount reasons follow invalid_amount → below_minimum →
// exceeds_budget. Every case is a refusal, so nothing may be persisted.
func TestSubmitSealedRejectOrder(t *testing.T) {
	t.Run("a submission with no live run is window_closed even for an impossible amount", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")

		// The amount is deliberately one that would read invalid_amount if the
		// amount were examined first (D-112 #2 before #3).
		rejection := env.sealedRefusal(t, an.PlayerID, -1)
		if rejection.Reason != ReasonWindowClosed {
			t.Errorf("reason = %q, want window_closed — the window is checked BEFORE the amount (D-112 #2/#3)", rejection.Reason)
		}
	})

	t.Run("a live ENGLISH run refuses a sealed submission with window_closed", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		mustOpenLiveRun(t, env, englishFixture()) // a LIVE run of the wrong TYPE

		rejection := env.sealedRefusal(t, an.PlayerID, 150_000)
		if rejection.Reason != ReasonWindowClosed {
			t.Fatalf("reason=%q, want window_closed: the run's TYPE decides (D-112 #2)", rejection.Reason)
		}
	})

	t.Run("the window is [started_at, deadline_at)", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		it := sealedFixture()
		mustOpenSealedRun(t, env, it)
		deadline := env.clock.now().UnixMilli() + it.durationMS

		// One millisecond before the deadline is still inside the window — a
		// late joiner's submission is not special-cased (SEAL-01).
		env.clock.t = time.UnixMilli(deadline - 1)
		env.mustSubmitSealed(t, an.PlayerID, 120_000)

		// The deadline itself is closed, even for an otherwise valid amount.
		env.clock.t = time.UnixMilli(deadline)
		if rejection := env.sealedRefusal(t, an.PlayerID, 130_000); rejection.Reason != ReasonWindowClosed {
			t.Errorf("reason = %q at the deadline, want window_closed (D-112 #3)", rejection.Reason)
		}
	})

	t.Run("a non-positive amount is invalid_amount even though it is also below the start price", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		mustOpenSealedRun(t, env, sealedFixture())

		for _, amount := range []int64{0, -1} {
			if rejection := env.sealedRefusal(t, an.PlayerID, amount); rejection.Reason != ReasonInvalidAmount {
				t.Errorf("amount %d: reason=%q, want invalid_amount before every bound (D-112 #3)", amount, rejection.Reason)
			}
		}
	})

	t.Run("an amount below the start price is below_minimum and reports that floor", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		it := sealedFixture()
		mustOpenSealedRun(t, env, it)

		rejection := env.sealedRefusal(t, an.PlayerID, it.startPrice-1)
		if rejection.Reason != ReasonBelowMinimum {
			t.Fatalf("reason=%q, want below_minimum just under the start price (D-112 #4)", rejection.Reason)
		}
		// SEAL-02's lower bound is start_price — NOT the English ladder's next
		// bid, which is right for a sealed run only by accident.
		if rejection.MinNextBid == nil || *rejection.MinNextBid != it.startPrice {
			t.Errorf("min_next_bid = %v, want the item's own start price %d (D-112 #4/SEAL-02)", rejection.MinNextBid, it.startPrice)
		}
	})

	t.Run("the start price itself is accepted", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		it := sealedFixture()
		mustOpenSealedRun(t, env, it)

		env.mustSubmitSealed(t, an.PlayerID, it.startPrice)
	})

	t.Run("an amount above the budget is exceeds_budget", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		mustOpenSealedRun(t, env, sealedFixture())
		mustSetBudget(t, env, an.PlayerID, 120_000)

		rejection := env.sealedRefusal(t, an.PlayerID, 120_001)
		if rejection.Reason != ReasonExceedsBudget {
			t.Fatalf("reason=%q, want exceeds_budget one above the budget (D-112 #5)", rejection.Reason)
		}
		if rejection.Budget != 120_000 {
			t.Errorf("refusal budget = %d, want the player's own 120000", rejection.Budget)
		}

		// The budget itself is affordable: the bound is inclusive (SEAL-02).
		env.mustSubmitSealed(t, an.PlayerID, 120_000)
	})

	t.Run("an amount below the start price AND above the budget reads below_minimum", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		it := sealedFixture()
		mustOpenSealedRun(t, env, it)
		// A budget below the start price, so one amount can violate both bounds.
		mustSetBudget(t, env, an.PlayerID, it.startPrice/2)

		if rejection := env.sealedRefusal(t, an.PlayerID, it.startPrice-10_000); rejection.Reason != ReasonBelowMinimum {
			t.Errorf("reason=%q, want below_minimum before exceeds_budget (D-112 #4 before #5)", rejection.Reason)
		}
	})
}

// TestSubmitSealedRejectChangesNothing pins D-53's non-mutation rule for the
// sealed flow: one refusal is answered exactly once, privately, and leaves the
// revision, the fan-out, the effective rows and the audit trail untouched.
func TestSubmitSealedRejectChangesNothing(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	it := sealedFixture()
	mustOpenSealedRun(t, env, it)

	// One accepted submission first, so "unchanged" is a comparison against real
	// rows rather than against nothing at all.
	env.mustSubmitSealed(t, an.PlayerID, 150_000)
	rowBefore, ok := env.sealedRowOf(t, it.id, an.PlayerID)
	if !ok {
		t.Fatal("the fixture's accepted submission left no effective row")
	}

	revBefore := env.engine.Rev()
	fanoutBefore := len(env.revs())
	eventsBefore := len(env.lastEvents())
	sheetsBefore, err := env.store.LoadSealedBids()
	if err != nil {
		t.Fatalf("LoadSealedBids: %v", err)
	}
	trailBefore, err := env.store.LoadBids()
	if err != nil {
		t.Fatalf("LoadBids: %v", err)
	}

	if rejection := env.sealedRefusal(t, an.PlayerID, 0); rejection.Reason != ReasonInvalidAmount {
		t.Errorf("reason = %q, want invalid_amount", rejection.Reason)
	}

	if got := env.engine.Rev(); got != revBefore {
		t.Errorf("revision = %d after a refusal, want the unchanged %d (D-53)", got, revBefore)
	}
	if got := len(env.revs()); got != fanoutBefore {
		t.Errorf("the refusal published %d revision(s), want the unchanged %d — a refusal has no fan-out (D-53)", got, fanoutBefore)
	}
	if rowAfter, ok := env.sealedRowOf(t, it.id, an.PlayerID); !ok || rowAfter != rowBefore {
		t.Errorf("effective row = %+v (present=%v) after a refusal, want the unchanged %+v", rowAfter, ok, rowBefore)
	}
	sheetsAfter, err := env.store.LoadSealedBids()
	if err != nil {
		t.Fatalf("LoadSealedBids after the refusal: %v", err)
	}
	if len(sheetsAfter) != len(sheetsBefore) {
		t.Errorf("sealed_bids holds %d row(s) after a refusal, want the unchanged %d", len(sheetsAfter), len(sheetsBefore))
	}
	trailAfter, err := env.store.LoadBids()
	if err != nil {
		t.Fatalf("LoadBids after the refusal: %v", err)
	}
	if len(trailAfter) != len(trailBefore) {
		t.Errorf("the audit trail holds %d row(s) after a refusal, want the unchanged %d (D-109)", len(trailAfter), len(trailBefore))
	}
	// The event list belongs to the last COMMITTED mutation, so "unchanged" is
	// the assertion: a refusal that recorded a frame would have grown it — and
	// the fan-out counter would have moved with it (D-53).
	if events := env.lastEvents(); len(events) != eventsBefore {
		t.Errorf("the refusal recorded %d event(s), want the unchanged %d: a refusal is not a mutation (D-53)", len(events), eventsBefore)
	}
}

// TestSubmitSealedLateJoinStaysEligible is SEAL-01's late-joiner rule: joining
// after the run opened does not touch the window, so a late arrival can still
// submit while the deadline is in the future — and the count then reports the
// larger room.
func TestSubmitSealedLateJoinStaysEligible(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	it := sealedFixture()
	mustOpenSealedRun(t, env, it) // the window opens with nobody in the room
	mustJoin(t, env, "An")

	env.clock.t = env.clock.t.Add(10 * time.Second) // ten seconds into the window
	late := mustJoin(t, env, "Binh")                // a late arrival, still inside it

	env.mustSubmitSealed(t, late.PlayerID, 130_000)
	if progress := env.sealedConfirmationOf(t, late.PlayerID); progress.Submitted != 1 || progress.Total != 2 {
		t.Errorf("sealed.received = %+v, want {1 2} — one sheet out of the room the join just grew", progress)
	}
}

// TestSubmitSealedPersistsOneEffectiveRow is SEAL-01's engine half: one real
// submission is ONE mutation — revision +1, one durable effective row, one
// audit-only `sealed_edit` row in the same transaction — and it answers the
// submitter with a private confirmation that carries the count and no amount.
func TestSubmitSealedPersistsOneEffectiveRow(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	it := sealedFixture()
	mustOpenSealedRun(t, env, it)

	revBefore := env.engine.Rev()
	now := env.clock.now().UnixMilli()

	rejection, refused, err := env.submitSealed(an.PlayerID, 150_000)
	if err != nil {
		t.Fatalf("SubmitSealed: %v", err)
	}
	if refused {
		t.Fatalf("a valid sealed bid was refused: %+v", rejection)
	}

	// D-110: one submit is one mutation, even though the effective row is an
	// upsert — the projector's X/Y counter has to move on every submit.
	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision = %d after one sealed submit, want %d (D-110/D-54)", got, revBefore+1)
	}

	row, ok := env.sealedRowOf(t, it.id, an.PlayerID)
	if !ok {
		t.Fatalf("no effective sealed row for (%s, %s) after a valid submit", it.id, an.PlayerID)
	}
	if row.amount != 150_000 {
		t.Errorf("effective amount = %d, want 150000", row.amount)
	}
	if row.submittedAt != now {
		t.Errorf("submitted_at = %d, want the first submission's %d (D-108)", row.submittedAt, now)
	}
	if row.updatedAt != now {
		t.Errorf("updated_at = %d, want %d", row.updatedAt, now)
	}

	// The durable half: the effective row AND its audit row, in one commit.
	sheets, err := env.store.LoadSealedBids()
	if err != nil {
		t.Fatalf("LoadSealedBids: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("sealed_bids holds %d row(s) after one submit, want exactly 1: %+v", len(sheets), sheets)
	}
	if sheets[0].Amount != 150_000 || sheets[0].SubmittedAt != now || sheets[0].UpdatedAt != now {
		t.Errorf("durable sealed row = %+v, want amount 150000 submitted/updated at %d", sheets[0], now)
	}

	trail, err := env.store.LoadBids()
	if err != nil {
		t.Fatalf("LoadBids: %v", err)
	}
	if len(trail) != 1 {
		t.Fatalf("the audit trail holds %d row(s) after one submit, want exactly 1: %+v", len(trail), trail)
	}
	if trail[0].Kind != "sealed_edit" {
		t.Errorf("audit kind = %q, want sealed_edit (D-109's vocabulary)", trail[0].Kind)
	}
	if trail[0].Amount != 150_000 || trail[0].ItemID != it.id || trail[0].PlayerID != an.PlayerID {
		t.Errorf("audit row = %+v, want the submitter's amount on this item", trail[0])
	}

	// D-111: the private confirmation is a count and nothing else, and it is
	// addressed to the submitter alone; the snapshot stays LAST (D-41).
	progress := env.sealedConfirmationOf(t, an.PlayerID)
	if progress.Submitted != 1 || progress.Total != 1 {
		t.Errorf("sealed.received = %+v, want {1 1} — one sheet out of one player", progress)
	}
	events := env.lastEvents()
	if len(events) != 2 {
		t.Fatalf("the submit emitted %d event(s), want the private confirmation then the snapshot: %+v", len(events), events)
	}
	if events[0].Type != EventSealedReceived || events[0].To != an.PlayerID {
		t.Errorf("first event = %+v, want a private sealed.received for %s", events[0], an.PlayerID)
	}
	if events[1].Type != EventSnapshot {
		t.Errorf("second event = %q, want the trailing state.snapshot (D-41)", events[1].Type)
	}
}

// TestSealedBidEditKeepsFirstSubmission is SEAL-02/SEAL-05's edit contract: three
// submissions leave ONE effective row whose amount is the LAST one, whose
// `submitted_at` is still the FIRST instant, whose `updated_at` is the last, and
// whose count is still one — while the audit keeps one row per request.
func TestSealedBidEditKeepsFirstSubmission(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	it := sealedFixture()
	mustOpenSealedRun(t, env, it)

	revBefore := env.engine.Rev()
	first := env.clock.now().UnixMilli()

	// The edit drill (PLAN §11): three different amounts, one row at the end.
	instants := make([]int64, 0, 3)
	for i, amount := range []int64{150_000, 180_000, 120_000} {
		instants = append(instants, env.clock.now().UnixMilli())
		if _, refused, err := env.submitSealed(an.PlayerID, amount); err != nil || refused {
			t.Fatalf("submit %d (%d) refused=%v err=%v, want a clean edit", i+1, amount, refused, err)
		}
		env.clock.t = env.clock.t.Add(4 * time.Second)
	}
	last := instants[2]

	row, ok := env.sealedRowOf(t, it.id, an.PlayerID)
	if !ok {
		t.Fatalf("no effective sealed row for (%s, %s) after three edits", it.id, an.PlayerID)
	}
	if row.amount != 120_000 {
		t.Errorf("effective amount = %d after three edits, want the LAST amount 120000 (SEAL-02)", row.amount)
	}
	if row.submittedAt != first {
		t.Errorf("submitted_at = %d after three edits, want the FIRST submission's %d — an edit must never move the tie-break instant (D-108/SEAL-05)", row.submittedAt, first)
	}
	if row.updatedAt != last {
		t.Errorf("updated_at = %d after three edits, want the last edit's %d", row.updatedAt, last)
	}

	sheets, err := env.store.LoadSealedBids()
	if err != nil {
		t.Fatalf("LoadSealedBids: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("sealed_bids holds %d row(s) after three edits, want exactly 1 (D-108): %+v", len(sheets), sheets)
	}
	if sheets[0].Amount != 120_000 || sheets[0].SubmittedAt != first || sheets[0].UpdatedAt != last {
		t.Errorf("durable row = %+v, want amount 120000, submitted_at %d, updated_at %d", sheets[0], first, last)
	}

	trail, err := env.store.LoadBids()
	if err != nil {
		t.Fatalf("LoadBids: %v", err)
	}
	if len(trail) != 3 {
		t.Fatalf("the audit trail holds %d row(s) after three submits, want exactly 3 (D-109): %+v", len(trail), trail)
	}
	for i, want := range []int64{150_000, 180_000, 120_000} {
		if trail[i].Kind != "sealed_edit" || trail[i].Amount != want {
			t.Errorf("audit row %d = %+v, want kind sealed_edit at %d", i, trail[i], want)
		}
	}

	// Three requests, one mutation each, and the count the projector reads is
	// still one (D-88/D-110).
	if got := env.engine.Rev(); got != revBefore+3 {
		t.Errorf("revision = %d after three submits, want %d", got, revBefore+3)
	}
	if progress := env.sealedConfirmationOf(t, an.PlayerID); progress.Submitted != 1 {
		t.Errorf("sealed.received.submitted = %d after three edits, want 1 — one row, one sheet (D-108)", progress.Submitted)
	}
}
