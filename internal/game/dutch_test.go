package game

import (
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"auctionlab/internal/store"
)

// This file is DTCH-01/DTCH-02's contract: the ONE Dutch price derivation
// (D-91), the serialized accept that settles at the server's own number
// (D-95…D-98) and the budget boundary that keeps a refusal a non-mutation.
//
// Time is data here (D-38): every case moves the injected clock instead of
// sleeping, so a Dutch clock is verified in microseconds and the expected price
// is an exact literal.

// dutchFixture is one complete Dutch item: 0 < floor < start with a positive
// step and a positive interval — exactly the mechanics D-92 requires before a
// Dutch row may go LIVE.
func dutchFixture() *item {
	return &item{
		id: "it-d1", orderIndex: 0, name: "Đồng hồ", description: "Dutch lot",
		auctionType: "dutch", truthValue: fixtureTruthValue,
		hintMin: 60_000, hintMax: 120_000,
		startPrice: 100_000, floorPrice: 40_000,
		decrementStep: 10_000, decrementIntervalMS: 1_000,
		status: "live",
	}
}

// mustOpenDutchRun installs the item and opens a DEADLINE-LESS run on it (D-93).
// Like mustOpenLiveRun it seeds the item row through the store write set first —
// the row is a real foreign-key target — and opens the run through Apply, the
// one production mutation path, so the run is a real committed run rather than a
// struct the test handed itself.
func mustOpenDutchRun(t *testing.T, env *testEnv, it *item) {
	t.Helper()

	now := env.clock.now().UnixMilli()
	if err := env.store.Commit(store.Mutation{
		Phase:     string(env.engine.Phase()),
		Revision:  env.engine.Rev(),
		UpdatedAt: now,
		Items:     []store.ItemRow{itemRow(it)},
	}); err != nil {
		t.Fatalf("seeding the Dutch item row: %v", err)
	}

	if err := env.engine.Apply(func(s *state) error {
		s.items[it.id] = it
		s.itemOrder = append(s.itemOrder, it.id)
		s.currentItemID = it.id
		s.phase = PhaseLive
		s.currentAuction = &auctionRun{itemID: it.id, startedAt: now} // no deadline (D-93)
		return nil
	}); err != nil {
		t.Fatalf("opening a live Dutch run: %v", err)
	}
}

// acceptDutch calls AcceptDutch and captures the private refusal, if any — the
// Dutch twin of placeBid (D-53/D-96).
func (env *testEnv) acceptDutch(playerID string) (BidRejection, bool, error) {
	var (
		rejection BidRejection
		got       bool
	)
	err := env.engine.AcceptDutch(playerID, func(_ View, r BidRejection) {
		rejection, got = r, true
	})
	return rejection, got, err
}

// TestDutchPrice is DTCH-01's contract: D-91's formula is exact at every step
// boundary, clamped at zero elapsed, pinned at the floor, total for a floor that
// equals the start, total for invalid mechanics, and it cannot be overflowed by
// a hostile elapsed into a price that is neither the start nor the floor.
func TestDutchPrice(t *testing.T) {
	base := &item{
		auctionType: "dutch", startPrice: 100_000, floorPrice: 40_000,
		decrementStep: 10_000, decrementIntervalMS: 1_000,
	}
	const started = int64(1_000_000)

	cases := []struct {
		name      string
		it        *item
		startedAt int64
		now       int64
		want      int64
	}{
		{
			name:      "one millisecond before the first interval keeps the start price",
			it:        base,
			startedAt: started, now: started + 999, want: 100_000,
		},
		{
			name:      "elapsed at exactly k intervals has stepped exactly k times",
			it:        base,
			startedAt: started, now: started + 3_000, want: 70_000,
		},
		{
			name:      "one millisecond short of the next step has stepped k-1 times",
			it:        base,
			startedAt: started, now: started + 3_999, want: 70_000,
		},
		{
			name:      "the instant the floor is reached the price is exactly the floor",
			it:        base,
			startedAt: started, now: started + 6_000, want: 40_000,
		},
		{
			name:      "long past floor time the price stays pinned at the floor",
			it:        base,
			startedAt: started, now: started + 600_000, want: 40_000,
		},
		{
			name: "a floor equal to the start stands still",
			it: &item{
				auctionType: "dutch", startPrice: 100_000, floorPrice: 100_000,
				decrementStep: 10_000, decrementIntervalMS: 1_000,
			},
			startedAt: started, now: started + 30_000, want: 100_000,
		},
		{
			name:      "a clock skew (now before started_at) clamps to zero elapsed",
			it:        base,
			startedAt: started, now: started - 5_000, want: 100_000,
		},
		{
			name: "a non-positive step returns the start price",
			it: &item{
				auctionType: "dutch", startPrice: 100_000, floorPrice: 40_000,
				decrementStep: 0, decrementIntervalMS: 1_000,
			},
			startedAt: started, now: started + 30_000, want: 100_000,
		},
		{
			name: "a non-positive interval returns the start price",
			it: &item{
				auctionType: "dutch", startPrice: 100_000, floorPrice: 40_000,
				decrementStep: 10_000, decrementIntervalMS: 0,
			},
			startedAt: started, now: started + 30_000, want: 100_000,
		},
		{
			name:      "a nil item derives zero rather than panicking",
			it:        nil,
			startedAt: started, now: started, want: 0,
		},
		{
			// A corrupt/legacy started_at of 0 with a 1 ms interval and the
			// largest legal step is what makes `steps × step` wrap int64: an
			// unclamped product lands OUTSIDE (floor, start) — here above the
			// start price, which D-91 forbids outright. The clamp must answer
			// the floor.
			name: "an elapsed far past the floor cannot overflow into a wrong price",
			it: &item{
				auctionType: "dutch", startPrice: 100_000, floorPrice: 40_000,
				decrementStep: MaxAmount, decrementIntervalMS: 1,
			},
			startedAt: 0, now: math.MaxInt64, want: 40_000,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dutchPriceAt(tc.it, tc.startedAt, tc.now); got != tc.want {
				t.Errorf("dutchPriceAt(started_at=%d, now=%d) = %d, want %d", tc.startedAt, tc.now, got, tc.want)
			}
		})
	}
}

// TestAcceptDutchSettlesAtTheDerivedPrice is DTCH-02's engine half: one accept
// settles the item ONCE at the instant's derived price, the winner pays exactly
// that number (budget − price + truth_value, total_value + truth_value), the
// audit row is committed with kind `accept` in the same mutation, and a second
// accept is refused without a second charge.
func TestAcceptDutchSettlesAtTheDerivedPrice(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	an := mustJoin(t, env, "An")
	it := dutchFixture()
	mustOpenDutchRun(t, env, it)

	// Three intervals in: the server's own number at this instant is 70_000.
	env.clock.t = env.clock.t.Add(3 * time.Second)
	const wantPrice = int64(70_000)
	acceptAt := env.clock.now().UnixMilli()

	revBefore := env.engine.Rev()
	budgetBefore := env.player(t, an.PlayerID).budget

	rejection, refused, err := env.acceptDutch(an.PlayerID)
	if err != nil {
		t.Fatalf("AcceptDutch = %v, want the item settled", err)
	}
	if refused {
		t.Fatalf("an accepted Dutch price was also refused: %+v", rejection)
	}

	// One mutation, one revision (D-54).
	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision = %d across an accept, want %d: one settle, one revision", got, revBefore+1)
	}

	// The reveal is the shared settle object (D-100), sold at the derived price.
	run := env.runOf(t)
	if run.settle == nil {
		t.Fatal("the accepted Dutch run has no settle reveal")
	}
	if run.settle.Outcome != "sold" {
		t.Errorf("settle outcome = %q, want sold", run.settle.Outcome)
	}
	if run.settle.Price != wantPrice {
		t.Errorf("settle price = %d, want the server-derived %d (DTCH-01)", run.settle.Price, wantPrice)
	}
	if run.settle.TruthValue != it.truthValue {
		t.Errorf("settle truth_value = %d, want %d", run.settle.TruthValue, it.truthValue)
	}
	if want := it.truthValue - wantPrice; run.settle.BudgetDelta != want {
		t.Errorf("settle budget_delta = %d, want truth_value − price = %d", run.settle.BudgetDelta, want)
	}
	if run.settle.Winner == nil || run.settle.Winner.Nickname != "An" {
		t.Errorf("settle winner = %+v, want An", run.settle.Winner)
	}

	// ECON-02's arithmetic, exactly once.
	p := env.player(t, an.PlayerID)
	if want := budgetBefore - wantPrice + it.truthValue; p.budget != want {
		t.Errorf("winner budget = %d, want %d (budget − price + truth_value)", p.budget, want)
	}
	if p.totalValue != it.truthValue {
		t.Errorf("winner total_value = %d, want += truth_value = %d", p.totalValue, it.truthValue)
	}

	// D-98: the accepted price is durable in the ONE audit trail, with the
	// accept kind — not a ladder bid.
	rows, err := env.store.LoadBids()
	if err != nil {
		t.Fatalf("loading the bid audit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("a Dutch accept appended %d audit row(s), want exactly 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.Kind != "accept" {
		t.Errorf("audit row kind = %q, want accept (D-98)", row.Kind)
	}
	if row.Amount != wantPrice {
		t.Errorf("audit row amount = %d, want the server-derived %d", row.Amount, wantPrice)
	}
	if row.PlayerID != an.PlayerID {
		t.Errorf("audit row player = %q, want %q", row.PlayerID, an.PlayerID)
	}
	if row.ItemID != it.id {
		t.Errorf("audit row item = %q, want %q", row.ItemID, it.id)
	}
	if row.CreatedAt != acceptAt {
		t.Errorf("audit row created_at = %d, want the accept instant %d", row.CreatedAt, acceptAt)
	}

	// The settle's sibling frames, in order (D-55/D-57).
	events := env.lastEvents()
	if len(events) != 3 || events[0].Type != EventItemSettled ||
		events[1].Type != EventLeaderboard || events[2].Type != EventSnapshot {
		t.Fatalf("the Dutch settle emitted %+v, want item.settled, leaderboard then snapshot", events)
	}

	// No phantom win: a second accept is refused and nothing moves again.
	revAfterSettle := env.engine.Rev()
	budgetAfterSettle := env.player(t, an.PlayerID).budget
	rejection, refused, err = env.acceptDutch(an.PlayerID)
	if err == nil || !refused {
		t.Fatalf("a second accept after the settle was not refused (err = %v, refusal = %+v)", err, rejection)
	}
	var refusedErr *BidRejectedError
	if !errors.As(err, &refusedErr) {
		t.Errorf("second accept error = %v, want a *BidRejectedError", err)
	}
	if got := env.engine.Rev(); got != revAfterSettle {
		t.Errorf("revision = %d after a refused accept, want the unchanged %d", got, revAfterSettle)
	}
	if got := env.player(t, an.PlayerID).budget; got != budgetAfterSettle {
		t.Errorf("budget = %d after a refused accept, want the unchanged %d: nobody is charged twice", got, budgetAfterSettle)
	}
}

// TestAcceptDutchBudget is DTCH-02's boundary: the price the server derives is
// what the budget check reads — equality is affordable, one VND above it is
// `exceeds_budget`, and the refusal carries the same derived number as the
// amount so the sentence the player reads names a real price.
func TestAcceptDutchBudget(t *testing.T) {
	t.Run("a price exactly equal to the budget is accepted", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		mustSetBudget(t, env, an.PlayerID, 70_000)
		it := dutchFixture()
		mustOpenDutchRun(t, env, it)
		env.clock.t = env.clock.t.Add(3 * time.Second)

		if _, refused, err := env.acceptDutch(an.PlayerID); err != nil || refused {
			t.Fatalf("an accept at exactly the budget was refused: err = %v, refusal = %+v", err, refused)
		}
		if got := env.player(t, an.PlayerID).budget; got != it.truthValue {
			t.Errorf("budget = %d, want 70_000 − 70_000 + truth_value = %d", got, it.truthValue)
		}
	})

	t.Run("one VND above the budget is exceeds_budget and changes nothing", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		an := mustJoin(t, env, "An")
		mustSetBudget(t, env, an.PlayerID, 69_999)
		it := dutchFixture()
		mustOpenDutchRun(t, env, it)
		env.clock.t = env.clock.t.Add(3 * time.Second)

		revBefore := env.engine.Rev()
		budgetBefore := env.player(t, an.PlayerID).budget
		eventsBefore := len(env.lastEvents())

		rejection, refused, err := env.acceptDutch(an.PlayerID)
		if !refused {
			t.Fatalf("an over-budget accept was not answered privately (err = %v)", err)
		}
		if rejection.Reason != ReasonExceedsBudget {
			t.Errorf("refusal reason = %q, want %q", rejection.Reason, ReasonExceedsBudget)
		}
		var refusedErr *BidRejectedError
		if !errors.As(err, &refusedErr) || refusedErr.Reason != ReasonExceedsBudget {
			t.Errorf("AcceptDutch error = %v, want a *BidRejectedError carrying %q", err, ReasonExceedsBudget)
		}
		if rejection.Amount != 70_000 {
			t.Errorf("refusal amount = %d, want the derived price at the accept instant (70000)", rejection.Amount)
		}
		if rejection.Budget != 69_999 {
			t.Errorf("refusal budget = %d, want the player's own 69999", rejection.Budget)
		}
		if rejection.MinNextBid != nil {
			t.Errorf("refusal min_next_bid = %d, want null: a Dutch price has no ladder (D-96)", *rejection.MinNextBid)
		}

		// A refusal is not a mutation (D-53/D-96).
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d after a refusal, want the unchanged %d", got, revBefore)
		}
		if got := env.player(t, an.PlayerID).budget; got != budgetBefore {
			t.Errorf("budget = %d after a refusal, want the unchanged %d", got, budgetBefore)
		}
		if got := len(env.lastEvents()); got != eventsBefore {
			t.Errorf("a refused accept left %d event(s) on the committed state, want the unchanged %d", got, eventsBefore)
		}
		if run := env.runOf(t); run.settle != nil {
			t.Errorf("a refused accept settled the run: %+v", run.settle)
		}
		rows, err := env.store.LoadBids()
		if err != nil {
			t.Fatalf("loading the bid audit: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("a refused accept persisted %d audit row(s), want none: %+v", len(rows), rows)
		}
	})
}
