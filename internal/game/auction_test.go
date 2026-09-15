package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"auctionlab/internal/store"
)

// This file is ENGL-02/ENGL-04/ENGL-05/ECON-02/ECON-03's contract: the ordered
// bid ladder, the anti-snipe extension formula and cap, and the one settle path
// shared by the deadline, a force-end and (in plan 03-02) the boot.
//
// Time is data here (D-38/D-60): every case moves env.clock instead of sleeping,
// so a 30-second auction is verified in microseconds and the arithmetic is an
// exact literal.

// mustJoin is the player-join helper: Phase 2 already proves the join contract,
// these tests only need identities.
func mustJoin(t *testing.T, env *testEnv, nickname string) JoinResult {
	t.Helper()
	res, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: nickname}, nil)
	if err != nil {
		t.Fatalf("join %s: %v", nickname, err)
	}
	return res
}

// mustSetBudget rewrites one player's budget through the mutation path, so the
// over-budget boundary can be tested at an exact number.
func mustSetBudget(t *testing.T, env *testEnv, playerID string, budget int64) {
	t.Helper()
	if err := env.engine.Apply(func(s *state) error {
		p := s.players[playerID]
		if p == nil {
			return fmt.Errorf("unknown player %q", playerID)
		}
		p.budget = budget
		return nil
	}); err != nil {
		t.Fatalf("setting %s's budget to %d: %v", playerID, budget, err)
	}
}

// englishFixture is one complete English item. Duration, increment and the
// anti-snipe settings are part of the type's contract (D-66), so every fixture
// carries them rather than relying on zero values.
func englishFixture() *item {
	return &item{
		id: "it-1", orderIndex: 0, name: "Bức tranh", description: "Sơn dầu",
		auctionType: "english", truthValue: fixtureTruthValue,
		hintMin: 200_000, hintMax: 500_000, startPrice: 100_000,
		minIncrement: 10_000, durationMS: 30_000,
		antiSnipeMode: "extend", antiSnipeWindowMS: 10_000, maxExtensions: 2,
		status: "live",
	}
}

// mustOpenLiveRun installs the item and opens a run on it. The items row is a
// real foreign-key target (game.current_item_id, bids.item_id,
// auction_runtime.item_id), so the fixture first commits that row through the
// store write set Task 1 built; the run itself opens through Apply — the one
// production mutation path — so the write-through this task implements is
// exercised, not bypassed.
func mustOpenLiveRun(t *testing.T, env *testEnv, it *item) {
	t.Helper()
	now := env.clock.now().UnixMilli()

	if err := env.store.Commit(store.Mutation{
		Phase:     string(env.engine.Phase()),
		Revision:  env.engine.Rev(),
		UpdatedAt: now,
		Items:     []store.ItemRow{gameItemRow(it)},
	}); err != nil {
		t.Fatalf("seeding the item row: %v", err)
	}

	if err := env.engine.Apply(func(s *state) error {
		s.items[it.id] = it
		s.itemOrder = append(s.itemOrder, it.id)
		s.currentItemID = it.id
		s.phase = PhaseLive
		s.currentAuction = &auctionRun{itemID: it.id, startedAt: now, deadlineAt: now + it.durationMS}
		return nil
	}); err != nil {
		t.Fatalf("opening a live run: %v", err)
	}
}

// gameItemRow maps the in-memory item to its durable row. It mirrors the
// engine's own write-set mapping and normalizes 0 to SQL NULL for the columns
// where 0 means "absent" (H3/the store's pointer contract).
func gameItemRow(it *item) store.ItemRow {
	opt := func(v int64) *int64 {
		if v == 0 {
			return nil
		}
		return &v
	}
	return store.ItemRow{
		ID: it.id, OrderIndex: it.orderIndex, Name: it.name, Description: it.description,
		ImageURL: it.imageURL, AuctionType: it.auctionType, TruthValue: it.truthValue,
		HintMin: opt(it.hintMin), HintMax: opt(it.hintMax), StartPrice: it.startPrice,
		MinIncrement: opt(it.minIncrement), FloorPrice: opt(it.floorPrice),
		DecrementStep: opt(it.decrementStep), DecrementIntervalMS: opt(it.decrementIntervalMS),
		DurationMS: opt(it.durationMS), AntiSnipeMode: it.antiSnipeMode,
		AntiSnipeWindowMS: opt(it.antiSnipeWindowMS), MaxExtensions: opt(it.maxExtensions),
		Status: it.status,
	}
}

// placeBid calls PlaceBid and captures the private rejection, if any (D-53).
func (env *testEnv) placeBid(playerID string, amount int64) (BidRejection, bool, error) {
	var (
		rejection BidRejection
		got       bool
	)
	err := env.engine.PlaceBid(playerID, amount, func(_ View, r BidRejection) {
		rejection, got = r, true
	})
	return rejection, got, err
}

// lastEvents returns the typed events of the most recent committed mutation.
func (env *testEnv) lastEvents() []Event {
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	return append([]Event(nil), env.engine.state.events...)
}

// playerOf reads one authoritative player row under the engine lock, so the
// assertions below are about the engine's own numbers, never a projection copy.
func (env *testEnv) player(t *testing.T, playerID string) *player {
	t.Helper()
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	p := env.engine.state.players[playerID]
	if p == nil {
		t.Fatalf("no player %q in the engine", playerID)
	}
	return p
}

// itemStatus reads the current status of one item row.
func (env *testEnv) itemStatus(itemID string) string {
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	it := env.engine.state.items[itemID]
	if it == nil {
		return ""
	}
	return it.status
}

// runOf reads the current run under the engine lock.
func (env *testEnv) runOf(t *testing.T) *auctionRun {
	t.Helper()
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	if env.engine.state.currentAuction == nil {
		t.Fatal("no auction run is open")
	}
	return env.engine.state.currentAuction
}

// requireRejection asserts one bid is refused with the exact D-52 reason, that
// the private callback carried the same reason, and that nothing moved.
func requireRejection(t *testing.T, env *testEnv, playerID string, amount int64, wantReason string) {
	t.Helper()

	revBefore := env.engine.Rev()
	rejection, got, err := env.placeBid(playerID, amount)
	if !got {
		t.Fatalf("bid %d by %s was not answered privately (err = %v), want the reason %q", amount, playerID, err, wantReason)
	}
	if rejection.Reason != wantReason {
		t.Errorf("rejection reason = %q, want %q", rejection.Reason, wantReason)
	}
	var refused *BidRejectedError
	if !errors.As(err, &refused) || refused.Reason != wantReason {
		t.Errorf("PlaceBid error = %v, want a *BidRejectedError carrying %q", err, wantReason)
	}
	if got := env.engine.Rev(); got != revBefore {
		t.Errorf("revision = %d after a rejection, want the unchanged %d: a rejection is not a mutation (D-53)", got, revBefore)
	}
	if events := env.lastEvents(); len(events) != 0 {
		t.Errorf("a rejected bid left %d event(s) on the committed state, want none", len(events))
	}
}

// requireAccepted asserts one bid is accepted with exactly one revision bump and
// exactly one bid.accepted event, and returns that event's payload.
func requireAccepted(t *testing.T, env *testEnv, playerID string, amount int64) BidAccepted {
	t.Helper()

	revBefore := env.engine.Rev()
	_, got, err := env.placeBid(playerID, amount)
	if err != nil {
		t.Fatalf("PlaceBid(%d) = %v, want an accepted bid", amount, err)
	}
	if got {
		t.Fatal("an accepted bid also produced a rejection callback")
	}
	if rev := env.engine.Rev(); rev != revBefore+1 {
		t.Errorf("revision = %d across an accepted bid, want %d: one mutation, one revision (D-54)", rev, revBefore+1)
	}
	events := env.lastEvents()
	if len(events) != 1 || events[0].Type != EventBidAccepted {
		t.Fatalf("an accepted bid emitted %+v, want exactly one %q event", events, EventBidAccepted)
	}
	payload, ok := events[0].Data.(BidAccepted)
	if !ok {
		t.Fatalf("the bid.accepted payload is %T, want BidAccepted", events[0].Data)
	}
	return payload
}

// TestBidValidationLadder is ENGL-02's ordered ladder (D-52). The ORDER is the
// contract, not an implementation detail: a late and too-low bid must read
// window_closed, and the leading bidder's bid must read already_leading only
// after it cleared the minimum and the budget.
func TestBidValidationLadder(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	binh := mustJoin(t, env, "Bình")
	mustSetBudget(t, env, binh.PlayerID, 110_000)

	it := englishFixture()
	it.antiSnipeMode = "disabled" // this test drives the clock directly
	mustOpenLiveRun(t, env, it)

	t.Run("a non-positive amount is invalid_amount", func(t *testing.T) {
		requireRejection(t, env, an.PlayerID, 0, ReasonInvalidAmount)
		requireRejection(t, env, an.PlayerID, -1, ReasonInvalidAmount)
	})

	t.Run("below the starting price is below_minimum", func(t *testing.T) {
		requireRejection(t, env, an.PlayerID, it.startPrice-1, ReasonBelowMinimum)
	})

	t.Run("the opening bid at the starting price is accepted", func(t *testing.T) {
		payload := requireAccepted(t, env, an.PlayerID, it.startPrice)
		if payload.CurrentPrice != it.startPrice {
			t.Errorf("bid.accepted current_price = %d, want %d", payload.CurrentPrice, it.startPrice)
		}
		wantMin := it.startPrice + it.minIncrement
		if payload.MinNextBid != wantMin {
			t.Errorf("bid.accepted min_next_bid = %d, want %d (server-computed, D-54)", payload.MinNextBid, wantMin)
		}
	})

	t.Run("below current + increment is below_minimum", func(t *testing.T) {
		requireRejection(t, env, binh.PlayerID, it.startPrice+it.minIncrement-1, ReasonBelowMinimum)
	})

	t.Run("the leading bidder cannot outbid themselves", func(t *testing.T) {
		requireRejection(t, env, an.PlayerID, it.startPrice+it.minIncrement, ReasonAlreadyLeading)
	})

	t.Run("over budget is exceeds_budget once the amount clears the minimum", func(t *testing.T) {
		// Bình's budget is 110_000 and the minimum is 110_000, so 110_001 is the
		// first amount that reaches the budget check — which is exactly the
		// boundary ECON-01 cares about.
		requireRejection(t, env, binh.PlayerID, 110_001, ReasonExceedsBudget)
	})

	t.Run("a bid equal to the budget is accepted", func(t *testing.T) {
		requireAccepted(t, env, binh.PlayerID, 110_000)
	})

	t.Run("the window check precedes below_minimum and already_leading", func(t *testing.T) {
		env.clock.t = testStart.Add(31 * time.Second) // deadline was testStart + 30s

		// An is not leading any more: if the ladder checked the minimum first,
		// this one would read below_minimum; it must read window_closed.
		requireRejection(t, env, an.PlayerID, 1, ReasonWindowClosed)
		// Bình IS leading: if the ladder checked that first, this would read
		// already_leading; the closed window still wins.
		requireRejection(t, env, binh.PlayerID, 1, ReasonWindowClosed)
		// …and a fully valid amount is still refused: the window is the wall.
		requireRejection(t, env, binh.PlayerID, 200_000, ReasonWindowClosed)
	})
}

// TestAntiSnipeExtend is ENGL-04's formula and cap (D-59): an accepted bid inside
// `remaining <= window` sets the deadline to now + window (never deadline +
// window), the count is persisted by D-50's column, and at the cap the bid is
// still accepted while the deadline stands still.
func TestAntiSnipeExtend(t *testing.T) {
	t.Run("extend sets the deadline to now + window and persists the count", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		binh := mustJoin(t, env, "Bình")
		it := englishFixture()
		mustOpenLiveRun(t, env, it)

		env.clock.t = testStart.Add(5 * time.Second)
		requireAccepted(t, env, an.PlayerID, it.startPrice)

		env.clock.t = testStart.Add(25 * time.Second) // remaining 5s <= window 10s
		requireAccepted(t, env, binh.PlayerID, 110_000)

		wantDeadline := testStart.Add(35 * time.Second).UnixMilli()
		run := env.runOf(t)
		if run.deadlineAt != wantDeadline {
			t.Errorf("deadline = %d, want %d (now + window, never deadline + window)", run.deadlineAt, wantDeadline)
		}
		if run.extensionsUsed != 1 {
			t.Errorf("extensions_used = %d, want 1", run.extensionsUsed)
		}

		// RT-06: the extension and the bid committed in the SAME transaction.
		rt, ok, err := env.store.LoadRuntime(it.id)
		if err != nil {
			t.Fatalf("LoadRuntime: %v", err)
		}
		if !ok {
			t.Fatal("the accepted bid did not persist a runtime row")
		}
		if rt.DeadlineAt == nil || *rt.DeadlineAt != wantDeadline {
			t.Errorf("persisted deadline_at = %v, want %d", rt.DeadlineAt, wantDeadline)
		}
		if rt.ExtensionsUsed != 1 {
			t.Errorf("persisted extensions_used = %d, want 1 (D-50: the cap must survive a crash)", rt.ExtensionsUsed)
		}
		bids, err := env.store.LoadBids()
		if err != nil {
			t.Fatalf("LoadBids: %v", err)
		}
		if len(bids) != 2 || bids[0].Amount != it.startPrice || bids[1].Amount != 110_000 {
			t.Errorf("persisted bids = %+v, want the two accepted amounts in order", bids)
		}
	})

	t.Run("the boundary is inclusive: remaining == window extends", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		it := englishFixture()
		mustOpenLiveRun(t, env, it)

		env.clock.t = testStart.Add(20 * time.Second) // remaining exactly the window
		requireAccepted(t, env, an.PlayerID, it.startPrice)

		if got := env.runOf(t).extensionsUsed; got != 1 {
			t.Errorf("extensions_used = %d at the inclusive boundary, want 1 (remaining <= window, D-59)", got)
		}
	})

	t.Run("a bid outside the window does not extend", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		it := englishFixture()
		mustOpenLiveRun(t, env, it)

		deadline := env.runOf(t).deadlineAt
		env.clock.t = testStart.Add(19 * time.Second) // remaining 11s > window 10s
		requireAccepted(t, env, an.PlayerID, it.startPrice)

		run := env.runOf(t)
		if run.extensionsUsed != 0 {
			t.Errorf("extensions_used = %d, want 0: a bid outside the final window must not extend", run.extensionsUsed)
		}
		if run.deadlineAt != deadline {
			t.Errorf("deadline = %d, want the untouched %d", run.deadlineAt, deadline)
		}
	})

	t.Run("anti_snipe_mode disabled leaves the deadline alone", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		it := englishFixture()
		it.antiSnipeMode = "disabled"
		mustOpenLiveRun(t, env, it)

		deadline := env.runOf(t).deadlineAt
		env.clock.t = testStart.Add(25 * time.Second)
		requireAccepted(t, env, an.PlayerID, it.startPrice)

		run := env.runOf(t)
		if run.extensionsUsed != 0 || run.deadlineAt != deadline {
			t.Errorf("disabled anti-snipe moved deadline/extensions to %d/%d, want %d/0", run.deadlineAt, run.extensionsUsed, deadline)
		}
	})

	t.Run("at the cap the bid is accepted and the deadline stands still", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		binh := mustJoin(t, env, "Bình")
		it := englishFixture() // max_extensions = 2
		mustOpenLiveRun(t, env, it)

		env.clock.t = testStart.Add(5 * time.Second)
		requireAccepted(t, env, an.PlayerID, 100_000)

		env.clock.t = testStart.Add(21 * time.Second)
		requireAccepted(t, env, binh.PlayerID, 110_000) // extension 1

		env.clock.t = testStart.Add(25 * time.Second)
		requireAccepted(t, env, an.PlayerID, 120_000) // extension 2

		cappedDeadline := testStart.Add(35 * time.Second).UnixMilli()
		if got := env.runOf(t).deadlineAt; got != cappedDeadline {
			t.Fatalf("deadline after two extensions = %d, want %d", got, cappedDeadline)
		}

		env.clock.t = testStart.Add(30 * time.Second)
		payload := requireAccepted(t, env, binh.PlayerID, 130_000) // at the cap

		run := env.runOf(t)
		if run.extensionsUsed != 2 {
			t.Errorf("extensions_used = %d after a capped bid, want the cap 2", run.extensionsUsed)
		}
		if run.deadlineAt != cappedDeadline {
			t.Errorf("deadline = %d after a capped bid, want the untouched %d (hard stop)", run.deadlineAt, cappedDeadline)
		}
		if payload.ExtensionsUsed != 2 {
			t.Errorf("bid.accepted extensions_used = %d, want 2", payload.ExtensionsUsed)
		}
		if payload.DeadlineAt == nil || *payload.DeadlineAt != cappedDeadline {
			t.Errorf("bid.accepted deadline_at = %v, want the unchanged %d", payload.DeadlineAt, cappedDeadline)
		}
	})
}

// TestSettlePaysTheWinnerOnce is ENGL-05/ECON-02 (D-55): the deadline trigger
// settles the top bidder at their own bid, the winner's economics move exactly
// once, the reveal is present at SETTLED, and a second trigger is a clean no-op.
func TestSettlePaysTheWinnerOnce(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	binh := mustJoin(t, env, "Bình")
	it := englishFixture()
	mustOpenLiveRun(t, env, it)

	env.clock.t = testStart.Add(5 * time.Second)
	requireAccepted(t, env, an.PlayerID, 100_000)
	env.clock.t = testStart.Add(10 * time.Second)
	requireAccepted(t, env, binh.PlayerID, 110_000)

	revBefore := env.engine.Rev()
	env.clock.t = testStart.Add(31 * time.Second)

	settled, err := env.engine.SettleIfDue()
	if err != nil {
		t.Fatalf("SettleIfDue: %v", err)
	}
	if !settled {
		t.Fatal("SettleIfDue reported no settlement after the deadline")
	}

	if got := env.engine.Phase(); got != PhaseSettled {
		t.Fatalf("phase after the deadline = %q, want %q", got, PhaseSettled)
	}
	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision = %d across a settle, want exactly one bump to %d", got, revBefore+1)
	}

	winner := env.player(t, binh.PlayerID)
	if want := int64(1_000_000 - 110_000 + fixtureTruthValue); winner.budget != want {
		t.Errorf("winner budget = %d, want budget - price + truth_value = %d", winner.budget, want)
	}
	if winner.totalValue != fixtureTruthValue {
		t.Errorf("winner total_value = %d, want += truth_value = %d", winner.totalValue, fixtureTruthValue)
	}
	loser := env.player(t, an.PlayerID)
	if loser.budget != 1_000_000 || loser.totalValue != 0 {
		t.Errorf("loser economics = budget %d / total %d, want 1000000/0 — only the winner pays", loser.budget, loser.totalValue)
	}
	if got := env.itemStatus(it.id); got != "settled" {
		t.Errorf("item status = %q after the settle, want settled", got)
	}

	run := env.runOf(t)
	if run.settle == nil {
		t.Fatal("the run carries no settle reveal after the settle")
	}
	if run.settle.Outcome != "sold" || run.settle.Price != 110_000 {
		t.Errorf("settle = %+v, want sold at 110000", run.settle)
	}
	if run.settle.Winner == nil || run.settle.Winner.Nickname != "Bình" {
		t.Errorf("settle winner = %+v, want Bình", run.settle.Winner)
	}
	if want := fixtureTruthValue - 110_000; run.settle.BudgetDelta != want {
		t.Errorf("settle budget_delta = %d, want truth_value - price = %d", run.settle.BudgetDelta, want)
	}

	t.Run("the settle fans out item.settled, leaderboard and a snapshot", func(t *testing.T) {
		events := env.lastEvents()
		wantTypes := []string{EventItemSettled, EventLeaderboard, EventSnapshot}
		if len(events) != len(wantTypes) {
			t.Fatalf("settle emitted %d event(s) (%+v), want %d", len(events), events, len(wantTypes))
		}
		for i, want := range wantTypes {
			if events[i].Type != want {
				t.Errorf("event %d type = %q, want %q", i, events[i].Type, want)
			}
		}
		item, ok := events[0].Data.(*PublicItem)
		if !ok {
			t.Fatalf("item.settled payload is %T, want *PublicItem", events[0].Data)
		}
		if item.Settle == nil || item.Settle.TruthValue != fixtureTruthValue {
			t.Errorf("item.settled settle object = %+v, want the reveal carrying truth_value", item.Settle)
		}
	})

	t.Run("the reveal reaches the player snapshot with the winner's values", func(t *testing.T) {
		raw, err := json.Marshal(env.engine.Snapshot(RolePlayer, an.PlayerID))
		if err != nil {
			t.Fatalf("marshaling the player snapshot: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decoding the player snapshot: %v", err)
		}
		item, ok := doc["item"].(map[string]any)
		if !ok {
			t.Fatalf("player snapshot item = %#v, want an object", doc["item"])
		}
		settle, ok := item["settle"].(map[string]any)
		if !ok {
			t.Fatalf("player snapshot item settled without a settle object: %s", raw)
		}
		if settle["outcome"] != "sold" || settle["price"] != float64(110_000) || settle["truth_value"] != float64(fixtureTruthValue) {
			t.Errorf("settle object = %#v, want sold/110000/%d", settle, fixtureTruthValue)
		}
		winner, ok := settle["winner"].(map[string]any)
		if !ok || winner["nickname"] != "Bình" {
			t.Errorf("settle.winner = %#v, want {nickname: Bình}", settle["winner"])
		}
	})

	t.Run("a second settle trigger is a clean no-op", func(t *testing.T) {
		revBefore := env.engine.Rev()
		settled, err := env.engine.SettleIfDue()
		if err != nil {
			t.Fatalf("second SettleIfDue: %v", err)
		}
		if settled {
			t.Error("a second SettleIfDue settled again: the LIVE guard must refuse (ECON-02 pays once)")
		}
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d across a refused settle, want %d", got, revBefore)
		}
	})

	t.Run("a bid after the settle is still window_closed and private", func(t *testing.T) {
		requireRejection(t, env, an.PlayerID, 200_000, ReasonWindowClosed)
	})

	t.Run("ForceEnd outside LIVE refuses without touching the state", func(t *testing.T) {
		revBefore := env.engine.Rev()
		if err := env.engine.ForceEnd(); !errors.Is(err, ErrNotLive) {
			t.Errorf("ForceEnd after SETTLED = %v, want ErrNotLive", err)
		}
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d across a refused force-end, want %d", got, revBefore)
		}
	})
}

// TestForceEndSettlesBeforeTheDeadline is D-55 (b): the presenter's force-end
// runs the same settle at a moment the clock has not reached.
func TestForceEndSettlesBeforeTheDeadline(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	it := englishFixture()
	mustOpenLiveRun(t, env, it)

	env.clock.t = testStart.Add(5 * time.Second)
	requireAccepted(t, env, an.PlayerID, 100_000)

	revBefore := env.engine.Rev()
	if err := env.engine.ForceEnd(); err != nil {
		t.Fatalf("ForceEnd: %v", err)
	}
	if got := env.engine.Phase(); got != PhaseSettled {
		t.Fatalf("phase after a force-end = %q, want %q", got, PhaseSettled)
	}
	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision = %d across a force-end, want %d", got, revBefore+1)
	}
	winner := env.player(t, an.PlayerID)
	if want := int64(1_000_000 - 100_000 + fixtureTruthValue); winner.budget != want {
		t.Errorf("winner budget after a force-end = %d, want %d", winner.budget, want)
	}
	if run := env.runOf(t); run.settle == nil || run.settle.Outcome != "sold" {
		t.Errorf("settle after a force-end = %+v, want a sold outcome", run.settle)
	}
	settled, err := env.engine.SettleIfDue()
	if err != nil || settled {
		t.Errorf("SettleIfDue after a force-end = %t/%v, want false/nil: the shared path settles once", settled, err)
	}
}

// TestUnsoldLeavesBudgetsAlone is ECON-03/D-69: no bids settles as a first-class
// `unsold` outcome — price 0, delta 0, an explicit null winner — and no player's
// budget or total value moves.
func TestUnsoldLeavesBudgetsAlone(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	binh := mustJoin(t, env, "Bình")
	mustSetBudget(t, env, binh.PlayerID, 777_777)
	it := englishFixture()
	mustOpenLiveRun(t, env, it)

	revBefore := env.engine.Rev()
	env.clock.t = testStart.Add(31 * time.Second)
	settled, err := env.engine.SettleIfDue()
	if err != nil {
		t.Fatalf("SettleIfDue: %v", err)
	}
	if !settled {
		t.Fatal("a no-bid LIVE item did not settle at its deadline")
	}
	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision = %d across an unsold settle, want %d", got, revBefore+1)
	}

	run := env.runOf(t)
	if run.settle == nil {
		t.Fatal("an unsold settlement carries no settle reveal")
	}
	if run.settle.Outcome != "unsold" || run.settle.Price != 0 || run.settle.BudgetDelta != 0 {
		t.Errorf("unsold settle = %+v, want outcome unsold with price 0 and delta 0", run.settle)
	}
	if run.settle.Winner != nil {
		t.Errorf("unsold settle winner = %+v, want nil (an explicit JSON null)", run.settle.Winner)
	}

	if got := env.player(t, an.PlayerID); got.budget != 1_000_000 || got.totalValue != 0 {
		t.Errorf("An's economics after an unsold settle = %d/%d, want 1000000/0", got.budget, got.totalValue)
	}
	if got := env.player(t, binh.PlayerID); got.budget != 777_777 || got.totalValue != 0 {
		t.Errorf("Bình's economics after an unsold settle = %d/%d, want 777777/0", got.budget, got.totalValue)
	}

	t.Run("the unsold reveal is on the wire with a null winner", func(t *testing.T) {
		raw, err := json.Marshal(env.engine.Snapshot(RoleScreen, ""))
		if err != nil {
			t.Fatalf("marshaling the screen snapshot: %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("decoding the screen snapshot: %v", err)
		}
		item, ok := doc["item"].(map[string]any)
		if !ok {
			t.Fatalf("screen snapshot item = %#v, want an object", doc["item"])
		}
		settle, ok := item["settle"].(map[string]any)
		if !ok {
			t.Fatalf("the unsold screen snapshot carries no settle object: %s", raw)
		}
		if settle["outcome"] != "unsold" {
			t.Errorf("settle.outcome = %#v, want unsold", settle["outcome"])
		}
		if winner, present := settle["winner"]; !present || winner != nil {
			t.Errorf("settle.winner = %#v (present=%t), want an explicit null", winner, present)
		}
		if settle["truth_value"] != float64(fixtureTruthValue) {
			t.Errorf("settle.truth_value = %#v, want the revealed %d", settle["truth_value"], fixtureTruthValue)
		}
	})
}
