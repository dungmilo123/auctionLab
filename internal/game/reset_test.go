package game

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// This file is ADMN-03's engine half (D-78) and ADMN-05's durable session reclaim
// (D-80).
//
// The reset fixture is a genuinely PLAYED game, not a hand-built state: two
// identities join, two items are configured, one run is taken all the way to
// SETTLED through the production mutation path, and a sealed-bid row is left
// behind the way Phase 6's submit path will leave it. A reset asserted against a
// lobby would prove nothing about removing a played round.

// startingBudgetOf reads the persisted budget a reset restores every player to.
func (env *testEnv) startingBudgetOf(t *testing.T) int64 {
	t.Helper()

	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	return env.engine.state.config.StartingBudget
}

// countRows counts one table on the fixture's own handle: a reset assertion is
// about what the database holds, never about an in-memory copy of it. The table
// name is a literal from this file, so the concatenation cannot be injected into.
func (env *testEnv) countRows(t *testing.T, table string) int {
	t.Helper()

	var n int
	if err := env.store.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

// trailLen reports the in-memory bid trail's length under the engine lock.
func (env *testEnv) trailLen() int {
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	return len(env.engine.state.bids)
}

// runOpen reports whether a live/settled run is still held in memory.
func (env *testEnv) runOpen() bool {
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	return env.engine.state.currentAuction != nil
}

// itemConfigOf copies one item with its status blanked, so a reset assertion can
// compare the whole configuration in one shot instead of restating every field:
// D-78 keeps the item config and "every other field" and rewrites only status.
func itemConfigOf(it *item) item {
	copied := *it
	copied.status = ""
	return copied
}

// insertSealedBid writes one sealed_bids row directly: Phase 6 owns the submit
// path, so a Phase 4 fixture has to leave the row exactly as that path will.
func insertSealedBid(t *testing.T, env *testEnv, itemID, playerID string, amount int64) {
	t.Helper()

	const insert = `
INSERT INTO sealed_bids (item_id, player_id, amount, submitted_at, updated_at)
VALUES (?, ?, ?, ?, ?)`
	now := env.clock.now().UnixMilli()
	if _, err := env.store.DB.Exec(insert, itemID, playerID, amount, now, now); err != nil {
		t.Fatalf("seeding a sealed bid: %v", err)
	}
}

// publishedFrames replaces the env's publisher with one that records every
// fan-out in order — the revision and the typed event list — so a reset test can
// assert the exact ordered frames the committed mutation produced (D-57). Both
// returned readers are safe to call from the test goroutine.
func (env *testEnv) publishedFrames() (events func() []Event, revs func() []int64) {
	var mu sync.Mutex
	var seen []Event
	var revisions []int64

	env.engine.SetPublisher(func(rev int64, _ View, list []Event) {
		mu.Lock()
		defer mu.Unlock()
		revisions = append(revisions, rev)
		seen = append(seen, list...)
	})

	return func() []Event {
			mu.Lock()
			defer mu.Unlock()
			return append([]Event(nil), seen...)
		}, func() []int64 {
			mu.Lock()
			defer mu.Unlock()
			return append([]int64(nil), revisions...)
		}
}

// TestResetKeepPlayers is D-78's keep_players contract end to end: one committed
// mutation returns a settled game to LOBBY, removes the bid trail, the sealed
// bids and the runtime row, restores every budget and total value, keeps every
// item's configuration and every identity, and emits exactly `phase.changed` +
// snapshot at exactly one new revision — with a restarted process observing the
// same persisted result.
func TestResetKeepPlayers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	base := testStart
	env := newTestEnvAt(t, path, &testClock{t: base})

	// --- the fixture: a played, settled round --------------------------------
	an := mustJoin(t, env, "An")
	binh := mustJoin(t, env, "Bình")
	first := mustUpsertItem(t, env, validEnglishInput())
	second := mustUpsertItem(t, env, validEnglishInput())

	mustOpenLive(t, env, first)
	mustBid(t, env, an.PlayerID, 100_000)
	env.clock.t = base.Add(5 * time.Second)
	mustBid(t, env, binh.PlayerID, 110_000)
	if err := env.engine.ForceEnd(); err != nil {
		t.Fatalf("force end: %v", err)
	}
	insertSealedBid(t, env, first, an.PlayerID, 250_000)

	// The fixture has to be a real played game, or the reset below asserts
	// nothing: the winner's economics have moved, the trail is two deep, and the
	// timing row the settle persisted is present.
	if got := env.itemStatus(first); got != "settled" {
		t.Fatalf("fixture item status = %q, want settled", got)
	}
	if got := env.itemStatus(second); got != "pending" {
		t.Fatalf("fixture second item status = %q, want pending", got)
	}
	if got := env.player(t, binh.PlayerID).budget; got != 1_240_000 {
		t.Fatalf("fixture winner budget = %d, want 1_240_000 (1.000.000 − 110.000 + 350.000)", got)
	}
	if got := env.player(t, binh.PlayerID).totalValue; got != 350_000 {
		t.Fatalf("fixture winner total_value = %d, want 350_000", got)
	}
	if got := env.trailLen(); got != 2 {
		t.Fatalf("fixture trail length = %d, want 2", got)
	}
	for _, tc := range []struct {
		table string
		want  int
	}{{"bids", 2}, {"sealed_bids", 1}, {"auction_runtime", 1}} {
		if got := env.countRows(t, tc.table); got != tc.want {
			t.Fatalf("fixture %s rows = %d, want %d", tc.table, got, tc.want)
		}
	}
	if got := env.startingBudgetOf(t); got != 1_000_000 {
		t.Fatalf("fixture starting budget = %d, want the bootstrapped 1.000.000", got)
	}

	// Everything the reset must preserve, captured before it runs.
	wantFirst := itemConfigOf(env.item(t, first))
	wantSecond := itemConfigOf(env.item(t, second))
	wantAn := *env.player(t, an.PlayerID)
	wantBinh := *env.player(t, binh.PlayerID)

	revBefore := env.engine.Rev()
	events, revs := env.publishedFrames()

	// --- the reset -----------------------------------------------------------
	if err := env.engine.Reset(ResetKeepPlayers); err != nil {
		t.Fatalf("Reset(keep_players): %v", err)
	}

	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision after the reset = %d, want %d: one mutation, one revision (D-35)", got, revBefore+1)
	}
	if got := revs(); len(got) != 1 || got[0] != revBefore+1 {
		t.Errorf("published revisions = %v, want exactly [%d]", got, revBefore+1)
	}

	// D-57/D-78: phase.changed first, then the role snapshots — and nothing else.
	frames := events()
	if len(frames) != 2 {
		t.Fatalf("reset published %d event(s) (%v), want exactly phase.changed + snapshot", len(frames), frames)
	}
	if frames[0].Type != EventPhaseChanged {
		t.Errorf("first frame = %q, want %q", frames[0].Type, EventPhaseChanged)
	}
	if frames[1].Type != EventSnapshot {
		t.Errorf("second frame = %q, want the role snapshot", frames[1].Type)
	}
	if changed, ok := frames[0].Data.(PhaseChanged); !ok {
		t.Errorf("phase.changed payload is %T, want PhaseChanged", frames[0].Data)
	} else {
		if changed.Phase != string(PhaseLobby) {
			t.Errorf("phase.changed carries %q, want LOBBY", changed.Phase)
		}
		if changed.Item != nil {
			t.Errorf("phase.changed carries item %+v, want an explicit null: a LOBBY has no current item", changed.Item)
		}
	}

	if got := env.engine.Phase(); got != PhaseLobby {
		t.Errorf("phase after the reset = %q, want LOBBY", got)
	}
	if env.runOpen() {
		t.Error("the reset left a run in memory; the timeline must be gone with the runtime row")
	}

	// The persisted game row moved with it, in the same transaction.
	var (
		phase     string
		itemID    string
		persisted int64
	)
	if err := env.store.DB.QueryRow(
		`SELECT phase, COALESCE(current_item_id, ''), revision FROM game WHERE id = 1`,
	).Scan(&phase, &itemID, &persisted); err != nil {
		t.Fatalf("reading the game row after the reset: %v", err)
	}
	if phase != string(PhaseLobby) {
		t.Errorf("persisted phase = %q, want LOBBY", phase)
	}
	if itemID != "" {
		t.Errorf("persisted current_item_id = %q, want empty (NULL)", itemID)
	}
	if persisted != revBefore+1 {
		t.Errorf("persisted revision = %d, want %d", persisted, revBefore+1)
	}

	// Nothing of the played round survives.
	if got := env.trailLen(); got != 0 {
		t.Errorf("the in-memory trail still holds %d bid(s) after the reset, want 0", got)
	}
	for _, tc := range []struct {
		table string
		want  int
	}{{"bids", 0}, {"sealed_bids", 0}, {"auction_runtime", 0}} {
		if got := env.countRows(t, tc.table); got != tc.want {
			t.Errorf("%s rows after the reset = %d, want %d", tc.table, got, tc.want)
		}
	}

	// The configuration is untouched and every item is playable again.
	if got := env.itemStatus(first); got != "pending" {
		t.Errorf("the settled item's status after the reset = %q, want pending", got)
	}
	if got := env.itemStatus(second); got != "pending" {
		t.Errorf("the pending item's status after the reset = %q, want pending", got)
	}
	if got := itemConfigOf(env.item(t, first)); got != wantFirst {
		t.Errorf("the settled item's configuration after the reset = %+v,\nwant the untouched %+v (D-78 keeps the item config)", got, wantFirst)
	}
	if got := itemConfigOf(env.item(t, second)); got != wantSecond {
		t.Errorf("the pending item's configuration after the reset = %+v,\nwant the untouched %+v", got, wantSecond)
	}

	// Identities, budgets and total values.
	if got := env.countRows(t, "players"); got != 2 {
		t.Errorf("players after a keep_players reset = %d, want 2", got)
	}
	for _, want := range []player{wantAn, wantBinh} {
		got := *env.player(t, want.id)
		if got.id != want.id || got.nickname != want.nickname || got.nicknameKey != want.nicknameKey {
			t.Errorf("player %q identity after the reset = %+v, want %+v", want.id, got, want)
		}
		if got.joinedAt != want.joinedAt {
			t.Errorf("player %q joined_at after the reset = %d, want the unchanged %d", want.id, got.joinedAt, want.joinedAt)
		}
		if got.budget != 1_000_000 {
			t.Errorf("player %q budget after the reset = %d, want the starting budget 1.000.000", want.id, got.budget)
		}
		if got.totalValue != 0 {
			t.Errorf("player %q total_value after the reset = %d, want 0", want.id, got.totalValue)
		}
	}

	// --- the restarted process observes the same reset ------------------------
	if err := env.store.Close(); err != nil {
		t.Fatalf("closing before the restart: %v", err)
	}
	restarted := newTestEnvAt(t, path, &testClock{t: base.Add(time.Hour)})

	if got := restarted.engine.Phase(); got != PhaseLobby {
		t.Errorf("phase after the restart = %q, want LOBBY", got)
	}
	if got := restarted.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision after the restart = %d, want the reset's %d", got, revBefore+1)
	}
	if got := restarted.engine.PlayerCount(); got != 2 {
		t.Errorf("players after the restart = %d, want the 2 kept identities", got)
	}
	for _, tc := range []struct {
		table string
		want  int
	}{{"bids", 0}, {"sealed_bids", 0}, {"auction_runtime", 0}, {"players", 2}, {"items", 2}} {
		if got := restarted.countRows(t, tc.table); got != tc.want {
			t.Errorf("%s rows after the restart = %d, want %d", tc.table, got, tc.want)
		}
	}
	if !restarted.store.Pragmas().ForeignKeys {
		t.Error("the restarted handle does not enforce foreign keys")
	}
}
