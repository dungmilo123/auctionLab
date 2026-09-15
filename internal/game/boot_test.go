package game

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file is RT-05's contract (D-62): a process that dies mid-auction is
// replaced by a process that resumes the SAME authoritative run from disk — or
// settles it exactly once, before any listener exists.
//
// Time is data here (D-38/D-60): every restart below is a NEW Engine on the same
// file at a SHIFTED instant. Reusing an engine or a clock would look like a
// restart while never exercising the load path at all (research Pitfall 14), and
// a sleep would make a 30-second auction cost 30 seconds to test.

// mustStartItem runs one `item.start` through the production path and fails the
// test on a refusal.
func mustStartItem(t *testing.T, env *testEnv, itemID string) {
	t.Helper()

	if err := env.engine.StartItem(itemID); err != nil {
		t.Fatalf("item.start %q: %v", itemID, err)
	}
}

// mustOpenLive drives the D-65 pair: LOBBY → ITEM_INTRO → LIVE, so the item's
// row and its auction_runtime row are written by the real mutation path.
func mustOpenLive(t *testing.T, env *testEnv, itemID string) {
	t.Helper()

	mustStartItem(t, env, itemID)
	mustStartItem(t, env, itemID)
}

// mustBid places one accepted bid (the private rejection callback is nil: a
// refusal here is a fixture error, not a case under test).
func mustBid(t *testing.T, env *testEnv, playerID string, amount int64) {
	t.Helper()

	if err := env.engine.PlaceBid(playerID, amount, nil); err != nil {
		t.Fatalf("bid %d by %s: %v", amount, playerID, err)
	}
}

// mustExtendOnce writes the drill's exact persisted shape: a first bid, then a
// second one inside the anti-snipe window, so the deadline becomes
// now + window (base+35s here) and extensions_used becomes 1 — neither of which
// is derivable from the item row alone (D-50/D-59). It returns the deadline.
func mustExtendOnce(t *testing.T, env *testEnv, base time.Time, an, binh string) int64 {
	t.Helper()

	env.clock.t = base.Add(5 * time.Second)
	mustBid(t, env, an, 100_000)
	env.clock.t = base.Add(25 * time.Second) // remaining 5s <= window 10s
	mustBid(t, env, binh, 110_000)           // deadline = T25 + 10s = T35, extensions_used = 1

	want := base.Add(35 * time.Second).UnixMilli()
	if got := env.runOf(t).deadlineAt; got != want {
		t.Fatalf("fixture deadline = %d, want %d (now + window, never deadline + window, D-59)", got, want)
	}
	return want
}

// topBid reads the run's leading bid under the engine lock.
func (env *testEnv) topBid(t *testing.T) (bid, bool) {
	t.Helper()

	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	return env.engine.state.topBid()
}

// TestRestartResumesLive is RT-05's resume arm: a process that died after an
// anti-snipe extend comes back with the phase, the item, the extended deadline,
// the extension count and the top bid exactly as they were persisted — with no
// revision movement, because nothing was settled — and a late joiner renders the
// whole run from one snapshot (SESS-04/D-63).
func TestRestartResumesLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	base := testStart

	// --- process 1: a live English item, one bid, then one anti-snipe extend --
	first := newTestEnvAt(t, path, &testClock{t: base})
	an := mustJoin(t, first, "An")
	binh := mustJoin(t, first, "Bình")
	itemID := mustUpsertItem(t, first, validEnglishInput())
	mustOpenLive(t, first, itemID)
	deadline := mustExtendOnce(t, first, base, an.PlayerID, binh.PlayerID)

	revBefore := first.engine.Rev()
	// A kill -9 drops the handle: there is no graceful shutdown path to call.
	if err := first.store.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}

	// --- process 2: the deadline is still ahead, so the run resumes -----------
	second := newTestEnvAt(t, path, &testClock{t: base.Add(30 * time.Second)})

	if got := second.engine.Phase(); got != PhaseLive {
		t.Fatalf("phase after a restart with time left = %s, want LIVE", got)
	}
	run := second.runOf(t)
	if run.itemID != itemID {
		t.Errorf("resumed item = %q, want the persisted %q", run.itemID, itemID)
	}
	if run.deadlineAt != deadline {
		t.Errorf("resumed deadline = %d, want the persisted %d", run.deadlineAt, deadline)
	}
	if run.extensionsUsed != 1 {
		t.Errorf("resumed extensions_used = %d, want 1 (D-50: it is stored, never derived)", run.extensionsUsed)
	}
	top, ok := second.topBid(t)
	if !ok {
		t.Fatal("no top bid after the restart: the run-scoped trail was not rebuilt from the bids table")
	}
	if top.playerID != binh.PlayerID || top.amount != 110_000 {
		t.Errorf("top bid after the restart = %s @ %d, want Bình @ 110000", top.playerID, top.amount)
	}
	if got := second.engine.Rev(); got != revBefore {
		t.Errorf("revision after a resume that settles nothing = %d, want the unchanged %d", got, revBefore)
	}
	if got := second.itemStatus(itemID); got != "live" {
		t.Errorf("item status after the restart = %q, want live", got)
	}

	// --- the late joiner renders the whole run from ONE snapshot --------------
	doc := decodeObject(t, second.engine.Snapshot(RolePlayer, binh.PlayerID))
	item := asObject(t, doc["item"], "item")
	for _, key := range []string{"current_price", "min_next_bid", "deadline_at", "top_bidder"} {
		if _, present := item[key]; !present {
			t.Errorf("the restarted LIVE item carries no %q, so a late joiner cannot render the run (SESS-04)", key)
		}
	}
	if item["current_price"] != float64(110_000) || item["deadline_at"] != float64(deadline) {
		t.Errorf("restarted item price/deadline = %#v/%#v, want 110000/%d", item["current_price"], item["deadline_at"], deadline)
	}
	if item["min_next_bid"] != float64(120_000) {
		t.Errorf("restarted min_next_bid = %#v, want 120000 (server-computed, D-54)", item["min_next_bid"])
	}
}

// TestExtensionsUsedSurvivesRestart is D-50's reason to exist: the anti-snipe
// cap must survive a crash exactly. Two extensions land at two instants, so the
// count is 2 and the deadline is the LAST now + window — values that deadline
// arithmetic cannot reconstruct.
func TestExtensionsUsedSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	base := testStart

	first := newTestEnvAt(t, path, &testClock{t: base})
	an := mustJoin(t, first, "An")
	binh := mustJoin(t, first, "Bình")
	itemID := mustUpsertItem(t, first, validEnglishInput())
	mustOpenLive(t, first, itemID)

	first.clock.t = base.Add(5 * time.Second)
	mustBid(t, first, an.PlayerID, 100_000)
	first.clock.t = base.Add(25 * time.Second) // remaining 5s <= window 10s
	mustBid(t, first, binh.PlayerID, 110_000)  // extension 1 → deadline T35
	first.clock.t = base.Add(27 * time.Second) // remaining 8s <= window 10s
	mustBid(t, first, an.PlayerID, 120_000)    // extension 2 → deadline T37

	if err := first.store.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}

	// --- process 2: still inside the SECOND extension's window ----------------
	second := newTestEnvAt(t, path, &testClock{t: base.Add(36 * time.Second)})

	run := second.runOf(t)
	if run.extensionsUsed != 2 {
		t.Errorf("extensions_used after the restart = %d, want 2 (D-50: the cap must survive a crash exactly)", run.extensionsUsed)
	}
	if want := base.Add(37 * time.Second).UnixMilli(); run.deadlineAt != want {
		t.Errorf("deadline after the restart = %d, want the last now + window %d (never deadline + window, D-59)", run.deadlineAt, want)
	}
	top, ok := second.topBid(t)
	if !ok || top.playerID != an.PlayerID || top.amount != 120_000 {
		t.Errorf("top bid after the restart = %+v/%t, want An @ 120000", top, ok)
	}
}

// TestRestartSettlesWhenDue is RT-05's settle arm: a deadline that passed while
// the process was down settles AT BOOT, exactly once, before anything can
// listen — and a second boot of the settled file pays nobody again (ECON-02).
func TestRestartSettlesWhenDue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	base := testStart

	first := newTestEnvAt(t, path, &testClock{t: base})
	an := mustJoin(t, first, "An")
	binh := mustJoin(t, first, "Bình")
	itemID := mustUpsertItem(t, first, validEnglishInput())
	mustOpenLive(t, first, itemID)
	deadline := mustExtendOnce(t, first, base, an.PlayerID, binh.PlayerID)
	revBefore := first.engine.Rev()
	if err := first.store.Close(); err != nil {
		t.Fatalf("closing the store: %v", err)
	}

	// --- process 2: the deadline passed while the process was down ------------
	second := newTestEnvAt(t, path, &testClock{t: base.Add(40 * time.Second)})

	if got := second.engine.Phase(); got != PhaseSettled {
		t.Fatalf("phase after a past-deadline boot = %s, want SETTLED", got)
	}
	if got := second.engine.Rev(); got != revBefore+1 {
		t.Errorf("the boot settle moved the revision by %d, want exactly 1", got-revBefore)
	}
	if got := second.itemStatus(itemID); got != "settled" {
		t.Errorf("item status after the boot settle = %q, want settled", got)
	}
	if got := second.runOf(t).deadlineAt; got != deadline {
		t.Errorf("settled run deadline = %d, want the persisted %d", got, deadline)
	}

	wantBudget := int64(1_000_000 - 110_000 + fixtureTruthValue)
	winner := second.player(t, binh.PlayerID)
	if winner.budget != wantBudget {
		t.Errorf("winner budget after the boot settle = %d, want budget − price + truth_value = %d", winner.budget, wantBudget)
	}
	if winner.totalValue != fixtureTruthValue {
		t.Errorf("winner total_value after the boot settle = %d, want += truth_value = %d", winner.totalValue, fixtureTruthValue)
	}
	if got := second.player(t, an.PlayerID); got.budget != 1_000_000 || got.totalValue != 0 {
		t.Errorf("loser economics after the boot settle = %d/%d, want the untouched 1000000/0", got.budget, got.totalValue)
	}

	// --- process 3: a second boot of the settled file must not pay again ------
	if err := second.store.Close(); err != nil {
		t.Fatalf("closing the settled store: %v", err)
	}
	third := newTestEnvAt(t, path, &testClock{t: base.Add(45 * time.Second)})

	if got := third.engine.Phase(); got != PhaseSettled {
		t.Errorf("phase on the second boot = %s, want the unchanged SETTLED", got)
	}
	if got := third.engine.Rev(); got != revBefore+1 {
		t.Errorf("a second boot re-settled: revision = %d, want the unchanged %d (ECON-02 pays once)", got, revBefore+1)
	}
	if got := third.player(t, binh.PlayerID).budget; got != wantBudget {
		t.Errorf("a second boot paid the winner again: budget = %d, want %d", got, wantBudget)
	}
}

// TestBootFailsHardOnBrokenRuntime is D-62's fail-hard half (T-03-04): durable
// state is trusted only after the phase↔item↔runtime triple checks out. A LIVE
// phase that cannot be reconstructed must abort the boot rather than fabricate a
// run, a deadline or a top bid.
func TestBootFailsHardOnBrokenRuntime(t *testing.T) {
	// persistedLive drives one extended LIVE auction on a fresh file and drops
	// the handle the way a kill -9 does, returning the file and the item id.
	persistedLive := func(t *testing.T) (path, itemID string) {
		t.Helper()

		path = filepath.Join(t.TempDir(), "auction.db")
		env := newTestEnvAt(t, path, &testClock{t: testStart})
		an := mustJoin(t, env, "An")
		binh := mustJoin(t, env, "Bình")
		itemID = mustUpsertItem(t, env, validEnglishInput())
		mustOpenLive(t, env, itemID)
		mustExtendOnce(t, env, testStart, an.PlayerID, binh.PlayerID)
		if err := env.store.Close(); err != nil {
			t.Fatalf("closing the store: %v", err)
		}
		return path, itemID
	}

	// corrupt applies one hand-edit to the persisted file, the shape a
	// half-written row or a future phase's mistake would take.
	corrupt := func(t *testing.T, path, stmt string) {
		t.Helper()

		st := openTestStore(t, path)
		if _, err := st.DB.Exec(stmt); err != nil {
			t.Fatalf("corrupting the file with %q: %v", stmt, err)
		}
		if err := st.Close(); err != nil {
			t.Fatalf("closing the corrupted file: %v", err)
		}
	}

	cases := []struct {
		name         string
		corrupt      func(t *testing.T, path string)
		wantContains string // empty means "the offending item id"
	}{
		{
			name:    "a LIVE phase whose runtime row is missing",
			corrupt: func(t *testing.T, path string) { corrupt(t, path, `DELETE FROM auction_runtime`) },
		},
		{
			name:    "a LIVE english item with a NULL deadline",
			corrupt: func(t *testing.T, path string) { corrupt(t, path, `UPDATE auction_runtime SET deadline_at = NULL`) },
		},
		{
			name:    "a LIVE phase whose item row is still intro",
			corrupt: func(t *testing.T, path string) { corrupt(t, path, `UPDATE items SET status = 'intro'`) },
		},
		{
			name:         "a LOBBY phase that still names a current item",
			corrupt:      func(t *testing.T, path string) { corrupt(t, path, `UPDATE game SET phase = 'LOBBY' WHERE id = 1`) },
			wantContains: "LOBBY",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, itemID := persistedLive(t)
			tc.corrupt(t, path)

			_, err := openEngineAt(t, path, testStart.Add(30*time.Second))
			if err == nil {
				t.Fatal("New accepted an inconsistent persisted auction; a LIVE state must fail hard rather than fabricate a run (D-62/T-03-04)")
			}
			want := tc.wantContains
			if want == "" {
				want = itemID
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("boot error %q does not name %q, so the operator cannot tell what is wrong", err, want)
			}
		})
	}
}
