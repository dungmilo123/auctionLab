package game

import (
	"path/filepath"
	"strings"
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

// rawToken reads the token the engine actually holds for one player, under the
// lock — the persisted value, never a projection copy (D-29).
func (env *testEnv) rawToken(t *testing.T, playerID string) string {
	t.Helper()

	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	p := env.engine.state.players[playerID]
	if p == nil {
		t.Fatalf("no player %q in the engine", playerID)
	}
	return p.token
}

// releasedTokenFor is the D-80 sentinel as the DATABASE must hold it, written as
// a hand-copied literal rather than read from the production constant: renaming
// that constant must not be able to move the persisted contract silently (the
// same convention the frozen wire lists use). NewToken emits 64 lowercase hex
// characters, so ':' can never appear in a minted token and the sentinel can
// never collide with one — including across two independently released rows,
// where an empty token would break players.token's UNIQUE index.
func releasedTokenFor(playerID string) string { return "released:" + playerID }

// adminPlayers projects the admin view's player list, so a reset assertion is
// about what the control room would actually receive.
func (env *testEnv) adminPlayers(t *testing.T) []AdminPlayer {
	t.Helper()

	view, ok := env.engine.Snapshot(RoleAdmin, "").(AdminView)
	if !ok {
		t.Fatal("the admin snapshot is not an AdminView")
	}
	return view.Players
}

// screenLobby projects the projector's lobby list.
func (env *testEnv) screenLobby(t *testing.T) []LobbyEntry {
	t.Helper()

	view, ok := env.engine.Snapshot(RoleScreen, "").(ScreenView)
	if !ok {
		t.Fatal("the screen snapshot is not a ScreenView")
	}
	return view.Lobby
}

// settledRound drives one complete English round in which `winner` buys the item
// at `price`, so a fixture can carry genuinely moved economics and a persisted
// runtime row. It returns the settled item's id.
func settledRound(t *testing.T, env *testEnv, winner string, price int64) string {
	t.Helper()

	itemID := mustUpsertItem(t, env, validEnglishInput())
	mustOpenLive(t, env, itemID)
	mustBid(t, env, winner, price)
	if err := env.engine.ForceEnd(); err != nil {
		t.Fatalf("forcing the fixture round to settle: %v", err)
	}
	return itemID
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
	wrote, err := env.engine.Reset(ResetKeepPlayers)
	if err != nil {
		t.Fatalf("Reset(keep_players): %v", err)
	}
	if !wrote {
		t.Error("Reset(keep_players) over a played room reported no write; the transport edge answers no-ops from that flag")
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

// TestResetClearPlayers is D-78's clear_players contract: the same reset plus
// every player row, deleted only after the tables that reference them are empty.
// It is also the one reset mode that is a real mutation in LOBBY — phones have
// joined by then, so "clear the room" always has something to do.
func TestResetClearPlayers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	base := testStart
	env := newTestEnvAt(t, path, &testClock{t: base})

	an := mustJoin(t, env, "An")
	binh := mustJoin(t, env, "Bình")
	first := settledRound(t, env, an.PlayerID, 100_000)
	insertSealedBid(t, env, first, binh.PlayerID, 250_000)

	// The fixture really carries dependent rows: the winner owns a bid and a
	// runtime row the delete order has to clear first.
	if got := env.countRows(t, "bids"); got != 1 {
		t.Fatalf("fixture bids rows = %d, want 1", got)
	}
	if got := env.countRows(t, "auction_runtime"); got != 1 {
		t.Fatalf("fixture auction_runtime rows = %d, want 1", got)
	}
	if got := env.countRows(t, "sealed_bids"); got != 1 {
		t.Fatalf("fixture sealed_bids rows = %d, want 1", got)
	}

	revBefore := env.engine.Rev()
	events, revs := env.publishedFrames()

	if wrote, err := env.engine.Reset(ResetClearPlayers); err != nil {
		t.Fatalf("Reset(clear_players): %v", err)
	} else if !wrote {
		t.Error("Reset(clear_players) reported no write; a lobby full of phones has a room to clear")
	}

	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision after a clear_players reset = %d, want %d", got, revBefore+1)
	}
	if got := revs(); len(got) != 1 || got[0] != revBefore+1 {
		t.Errorf("published revisions = %v, want exactly [%d]", got, revBefore+1)
	}
	frames := events()
	if len(frames) != 2 || frames[0].Type != EventPhaseChanged || frames[1].Type != EventSnapshot {
		t.Errorf("clear_players published %v, want exactly phase.changed + snapshot", frames)
	}
	if got := env.engine.Phase(); got != PhaseLobby {
		t.Errorf("phase after a clear_players reset = %q, want LOBBY", got)
	}

	// Nothing of the round survives — and neither does any identity.
	for _, tc := range []struct {
		table string
		want  int
	}{{"bids", 0}, {"sealed_bids", 0}, {"auction_runtime", 0}, {"players", 0}, {"items", 1}} {
		if got := env.countRows(t, tc.table); got != tc.want {
			t.Errorf("%s rows after the clear_players reset = %d, want %d", tc.table, got, tc.want)
		}
	}
	if got := env.engine.PlayerCount(); got != 0 {
		t.Errorf("in-memory players after a clear_players reset = %d, want 0", got)
	}
	if got := env.adminPlayers(t); len(got) != 0 {
		t.Errorf("the admin projection still lists %d player(s) after the reset, want none: %+v", len(got), got)
	}
	if got := env.screenLobby(t); len(got) != 0 {
		t.Errorf("the projector still lists %d lobby entr(ies) after the reset, want none", len(got))
	}
	// The item configuration survives, exactly as keep_players leaves it.
	if got := env.itemStatus(first); got != "pending" {
		t.Errorf("the settled item's status after the reset = %q, want pending", got)
	}
	if !env.store.Pragmas().ForeignKeys {
		t.Error("the fixture handle stopped enforcing foreign keys")
	}

	// --- a second clear_players in LOBBY is still a real mutation ------------
	two := mustJoin(t, env, "Chi")
	three := mustJoin(t, env, "Dũng")
	if got := env.engine.Phase(); got != PhaseLobby {
		t.Fatalf("phase before the LOBBY reset = %q, want LOBBY", got)
	}

	revBefore = env.engine.Rev()
	events, revs = env.publishedFrames()
	if wrote, err := env.engine.Reset(ResetClearPlayers); err != nil {
		t.Fatalf("clear_players in LOBBY: %v", err)
	} else if !wrote {
		t.Error("clear_players in LOBBY reported no write: the phones joined, so there is something to clear")
	}
	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision after a LOBBY clear_players = %d, want %d: the phones joined, so there is something to clear", got, revBefore+1)
	}
	if got := revs(); len(got) != 1 || got[0] != revBefore+1 {
		t.Errorf("published revisions for the LOBBY reset = %v, want exactly [%d]", got, revBefore+1)
	}
	if got := env.countRows(t, "players"); got != 0 {
		t.Errorf("players after the LOBBY clear_players = %d, want 0", got)
	}
	for _, id := range []string{two.PlayerID, three.PlayerID} {
		_, err := env.engine.Join(JoinRequest{Role: RolePlayer, Token: id}, nil)
		if err == nil {
			t.Errorf("player %q still resumes after clear_players", id)
		}
	}
}

// TestResetPhaseEdges is D-78's edge contract: the mode vocabulary is closed and
// an unknown mode changes nothing at all, FINALE → LOBBY is the only way out of
// the finale, and a keep_players reset of an already-fresh lobby is a genuine
// no-op rather than a redundant write.
func TestResetPhaseEdges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	env := newTestEnv(t, path)

	an := mustJoin(t, env, "An")
	first := mustUpsertItem(t, env, validEnglishInput())
	second := mustUpsertItem(t, env, validEnglishInput())

	// --- an unknown mode is refused before anything is touched ---------------
	revBefore := env.engine.Rev()
	phaseBefore := env.engine.Phase()
	events, revs := env.publishedFrames()

	wrote, err := env.engine.Reset(ResetMode("reset_everything"))
	if err == nil {
		t.Fatal("Reset(reset_everything) was accepted; the mode vocabulary must be closed")
	}
	if wrote {
		t.Error("a refused mode reported a write; nothing was committed")
	}
	if !strings.Contains(err.Error(), "reset_everything") {
		t.Errorf("refusal %q does not name the rejected mode, so the presenter cannot act on it", err)
	}
	if got := env.engine.Rev(); got != revBefore {
		t.Errorf("revision after a refused reset = %d, want the unchanged %d", got, revBefore)
	}
	if got := env.engine.Phase(); got != phaseBefore {
		t.Errorf("phase after a refused reset = %q, want the unchanged %q", got, phaseBefore)
	}
	if got := len(revs()); got != 0 {
		t.Errorf("a refused reset published %d fan-out(s), want none", got)
	}
	if got := len(events()); got != 0 {
		t.Errorf("a refused reset produced %d event(s), want none", got)
	}
	if got := env.countRows(t, "players"); got != 1 {
		t.Errorf("players rows after a refused reset = %d, want the unchanged 1", got)
	}

	// --- FINALE can be reset back to LOBBY ----------------------------------
	mustOpenLive(t, env, first)
	if err := env.engine.ForceEnd(); err != nil {
		t.Fatalf("force end: %v", err)
	}
	if err := env.engine.NextItem(); err != nil {
		t.Fatalf("next item: %v", err)
	}
	if err := env.engine.NextItem(); err != nil {
		t.Fatalf("next item again: %v", err)
	}
	if got := env.engine.Phase(); got != PhaseFinale {
		t.Fatalf("phase before the FINALE reset = %q, want FINALE", got)
	}

	if wrote, err := env.engine.Reset(ResetKeepPlayers); err != nil {
		t.Fatalf("Reset(keep_players) from FINALE: %v", err)
	} else if !wrote {
		t.Error("Reset(keep_players) from FINALE reported no write; a played game has a timeline to clear")
	}
	if got := env.engine.Phase(); got != PhaseLobby {
		t.Errorf("phase after a FINALE reset = %q, want LOBBY", got)
	}
	for _, id := range []string{first, second} {
		if got := env.itemStatus(id); got != "pending" {
			t.Errorf("item %q status after the FINALE reset = %q, want pending", id, got)
		}
	}
	if got := env.player(t, an.PlayerID).budget; got != 1_000_000 {
		t.Errorf("budget after the FINALE reset = %d, want the starting budget", got)
	}

	// --- a keep_players reset of an already-fresh lobby is a no-op ----------
	revBefore = env.engine.Rev()
	events, revs = env.publishedFrames()
	wrote, err = env.engine.Reset(ResetKeepPlayers)
	if err != nil {
		t.Fatalf("Reset(keep_players) in LOBBY: %v", err)
	}
	if wrote {
		t.Error("a no-op keep_players reset reported a write; nothing was committed")
	}
	if got := env.engine.Rev(); got != revBefore {
		t.Errorf("revision after a LOBBY keep_players reset = %d, want the unchanged %d: there is nothing to reset, so there is nothing to persist", got, revBefore)
	}
	if got := len(revs()); got != 0 {
		t.Errorf("a no-op reset published %d fan-out(s), want none", got)
	}
	if got := len(events()); got != 0 {
		t.Errorf("a no-op reset produced %d event(s), want none", got)
	}
	if got := env.itemStatus(first); got != "pending" {
		t.Errorf("item status after the no-op reset = %q, want the unchanged pending", got)
	}
}

// TestReleaseSession is D-80's release contract: releasing a player invalidates
// the token that device holds — for good — while every released row keeps a
// DISTINCT persisted value, because players.token is UNIQUE and an empty string
// would collide on the second release (research Pitfall 7).
func TestReleaseSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	base := testStart
	env := newTestEnvAt(t, path, &testClock{t: base})

	an := mustJoin(t, env, "An")
	env.clock.t = base.Add(time.Second)
	binh := mustJoin(t, env, "Bình")
	settledRound(t, env, an.PlayerID, 100_000)

	// An id the engine does not know is refused rather than silently ignored:
	// the command names a row, and a typo must not report success.
	if wrote, err := env.engine.ReleaseSession("no-such-player"); err == nil {
		t.Error("releasing an unknown player was accepted; the command must validate the row it names")
	} else if wrote {
		t.Error("a refused release reported a write; nothing was committed")
	}
	if _, err := env.engine.ReleaseSession(""); err == nil {
		t.Error("releasing the empty id was accepted")
	}

	// --- two independently released rows do not collide ---------------------
	revBefore := env.engine.Rev()
	if wrote, err := env.engine.ReleaseSession(an.PlayerID); err != nil {
		t.Fatalf("releasing the first player: %v", err)
	} else if !wrote {
		t.Error("releasing a live row reported no write; the token write is the command's whole point")
	}
	anReleased := env.rawToken(t, an.PlayerID)
	if wrote, err := env.engine.ReleaseSession(binh.PlayerID); err != nil {
		t.Fatalf("releasing a second player while the first is released: %v", err)
	} else if !wrote {
		t.Error("releasing the second live row reported no write")
	}
	binhReleased := env.rawToken(t, binh.PlayerID)

	if anReleased != releasedTokenFor(an.PlayerID) {
		t.Errorf("released token = %q, want the D-80 sentinel %q", anReleased, releasedTokenFor(an.PlayerID))
	}
	if binhReleased != releasedTokenFor(binh.PlayerID) {
		t.Errorf("second released token = %q, want the D-80 sentinel %q", binhReleased, releasedTokenFor(binh.PlayerID))
	}
	if anReleased == binhReleased {
		t.Error("two released rows hold the same token; players.token is UNIQUE and an empty sentinel cannot survive a second release")
	}
	if anReleased == "" || binhReleased == "" {
		t.Error("a released row was left with an empty token, which cannot recover from a crash mid-release")
	}
	if got := env.engine.Rev(); got != revBefore+2 {
		t.Errorf("revision after two releases = %d, want %d: each one is a durable player-row write", got, revBefore+2)
	}

	// --- the old token can never resume again -------------------------------
	for _, tc := range []struct {
		name  string
		token string
	}{
		{"the token the device still holds", an.Token},
		{"the sentinel itself, if a client guessed it", anReleased},
	} {
		t.Run(tc.name+" is token_invalid", func(t *testing.T) {
			_, err := env.engine.Join(JoinRequest{Role: RolePlayer, Token: tc.token}, nil)
			requireJoinCode(t, err, CodeTokenInvalid)
		})
	}

	// Releasing is idempotent: the row is already released, so there is nothing
	// left to write, the revision stands still, and the engine says so — the
	// hub uses that `false` to answer the presenter's panel with a snapshot.
	revBefore = env.engine.Rev()
	if wrote, err := env.engine.ReleaseSession(an.PlayerID); err != nil {
		t.Fatalf("releasing an already-released player: %v", err)
	} else if wrote {
		t.Error("re-releasing an already-released row reported a write; nothing was committed")
	}
	if got := env.engine.Rev(); got != revBefore {
		t.Errorf("revision after re-releasing = %d, want the unchanged %d", got, revBefore)
	}

	// --- the sentinel is persisted, and it cannot be minted -----------------
	rows, err := env.store.LoadPlayers()
	if err != nil {
		t.Fatalf("LoadPlayers: %v", err)
	}
	persisted := map[string]string{}
	for _, row := range rows {
		persisted[row.ID] = row.Token
	}
	if got := persisted[an.PlayerID]; got != releasedTokenFor(an.PlayerID) {
		t.Errorf("the store holds token %q for the released player, want the D-80 sentinel %q", got, releasedTokenFor(an.PlayerID))
	}

	// The sentinel is only safe because the mint can never produce a prefix
	// collision: 64 lowercase hex characters, and ':' is not one of them.
	for i := 0; i < 64; i++ {
		token, err := NewToken()
		if err != nil {
			t.Fatalf("NewToken: %v", err)
		}
		if strings.Contains(token, ":") {
			t.Fatalf("NewToken minted %q — the sentinel prefix is no longer impossible to mint", token)
		}
	}
}

// TestClaimReleasedNickname is D-80's reclaim contract: a nickname whose row was
// released is NOT taken — the next device to type it claims the very same row,
// keeping its id, budget, total value and joined_at, and receiving a normal
// minted token. A nickname whose row was never released stays reserved (D-32).
func TestClaimReleasedNickname(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	base := testStart
	env := newTestEnvAt(t, path, &testClock{t: base})

	an := mustJoin(t, env, "An")
	env.clock.t = base.Add(time.Second)
	binh := mustJoin(t, env, "Bình")
	env.clock.t = base.Add(2 * time.Second)
	settledRound(t, env, an.PlayerID, 100_000)

	before := *env.player(t, an.PlayerID)
	if before.budget != 1_250_000 || before.totalValue != 350_000 {
		t.Fatalf("fixture economics = budget %d / total %d, want 1.250.000 / 350.000", before.budget, before.totalValue)
	}

	if _, err := env.engine.ReleaseSession(an.PlayerID); err != nil {
		t.Fatalf("releasing %s: %v", an.PlayerID, err)
	}

	// The claim comes much later, so "joined_at unchanged" cannot be an accident
	// of the clock standing still.
	env.clock.t = base.Add(time.Hour)
	revBefore := env.engine.Rev()

	var hello Hello
	var projection any
	claim, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "  an  "}, func(h Hello, v any) {
		hello, projection = h, v
	})
	if err != nil {
		t.Fatalf("claiming the released nickname: %v", err)
	}

	if claim.PlayerID != an.PlayerID {
		t.Errorf("the claim minted player %q, want the released row %q claimed back", claim.PlayerID, an.PlayerID)
	}
	if claim.Resumed {
		t.Error("a nickname claim reported itself as a token resume; it is a fresh registration on an existing row")
	}
	if claim.Token == an.Token {
		t.Error("the claim handed back the old token, which the admin just invalidated")
	}
	if claim.Token == releasedTokenFor(an.PlayerID) {
		t.Error("the claim handed back the released sentinel instead of minting a token")
	}
	if got := len(claim.Token); got != 64 {
		t.Errorf("claimed token has %d characters, want a normal 64-character mint", got)
	}
	if got := env.engine.Rev(); got != revBefore+1 {
		t.Errorf("revision after the claim = %d, want %d: rewriting the row is a durable mutation", got, revBefore+1)
	}
	if hello.Token != claim.Token || hello.PlayerID != an.PlayerID {
		t.Errorf("hello after the claim = %+v, want the claimed id and the new token", hello)
	}
	if _, ok := projection.(PlayerView); !ok {
		t.Errorf("the claim's second emit argument is %T, want a PlayerView", projection)
	}

	// The row is the SAME player: id, nickname, economics and join time survive.
	after := env.player(t, an.PlayerID)
	if after.id != before.id || after.nickname != before.nickname || after.nicknameKey != before.nicknameKey {
		t.Errorf("claimed identity = %+v, want the unchanged %+v", after, before)
	}
	if after.budget != before.budget {
		t.Errorf("claimed budget = %d, want the retained %d", after.budget, before.budget)
	}
	if after.totalValue != before.totalValue {
		t.Errorf("claimed total_value = %d, want the retained %d", after.totalValue, before.totalValue)
	}
	if after.joinedAt != before.joinedAt {
		t.Errorf("claimed joined_at = %d, want the retained %d (the identity is not new)", after.joinedAt, before.joinedAt)
	}
	if after.token != claim.Token {
		t.Errorf("stored token = %q, want the minted %q", after.token, claim.Token)
	}

	// The minted token now resumes normally, and the row is no longer released.
	resumed, err := env.engine.Join(JoinRequest{Role: RolePlayer, Token: claim.Token}, nil)
	if err != nil {
		t.Fatalf("resuming with the claimed token: %v", err)
	}
	if !resumed.Resumed || resumed.PlayerID != an.PlayerID {
		t.Errorf("resume after the claim = %+v, want the claimed row resumed", resumed)
	}

	// A stale token alongside the nickname is still D-30 (2): the unknown token
	// is ignored and the nickname decides, so a device with a stale token is not
	// stranded.
	if _, err := env.engine.ReleaseSession(binh.PlayerID); err != nil {
		t.Fatalf("releasing %s: %v", binh.PlayerID, err)
	}
	withStale, err := env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "Bình", Token: binh.Token}, nil)
	if err != nil {
		t.Fatalf("claiming with the stale token still attached: %v", err)
	}
	if withStale.PlayerID != binh.PlayerID || withStale.Resumed {
		t.Errorf("stale-token claim = %+v, want the released row %q claimed as a fresh registration", withStale, binh.PlayerID)
	}

	// --- a nickname that was never released stays reserved (D-32) -----------
	env.clock.t = base.Add(2 * time.Hour)
	mustJoin(t, env, "Chi")
	_, err = env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "CHI"}, nil)
	requireJoinCode(t, err, CodeNicknameTaken)
	_, err = env.engine.Join(JoinRequest{Role: RolePlayer, Nickname: "ch\u200Bi"}, nil)
	requireJoinCode(t, err, CodeNicknameTaken)
}
