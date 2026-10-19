package store

import (
	"auctionlab/internal/testpg"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This file is ADMN-03's persistence half (D-78, T-04-05): the reset write set,
// the child-before-parent delete order that `PRAGMA foreign_keys(1)` makes
// load-bearing, and the proof that a reopened file observes the same result.
//
// The fixture is a PLAYED round — two identities, two items, a bid trail, a
// sealed bid and the runtime row a settle leaves behind — written through the
// same Commit the engine uses, so the test exercises the production write path
// instead of raw SQL. The one exception is the sealed bid: the engine does not
// own a submit path until Phase 6, so the row is inserted directly, exactly as
// that path will leave it.

// resetNow is the fixture's fixed instant, the same convention snapshot_test.go
// uses (D-38: time is data, never a sleep).
var resetNow = time.UnixMilli(1_700_000_000_000)

// tableRows counts one table. The name is a compile-time literal from this file,
// never caller input, so the concatenation cannot be injected into (the same
// note columnExists carries).
func tableRows(t *testing.T, st *Store, table string) int {
	t.Helper()

	var n int
	if err := st.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return n
}

// seedPlayedRound writes one finished game: two players (one of them already
// paid), two configured items (one settled, one still pending), two accepted
// bids, a sealed bid and the settled runtime row. It returns the two player
// rows as they were committed, so a later assertion can compare against them.
func seedPlayedRound(t *testing.T, st *Store) []PlayerRow {
	t.Helper()

	players := []PlayerRow{
		{
			ID: "p-an", Nickname: "An", NicknameKey: "an", Token: strings.Repeat("a", 64),
			Budget: 1_240_000, TotalValue: 350_000,
			JoinedAt: resetNow.UnixMilli(), LastSeenAt: resetNow.UnixMilli(),
		},
		{
			ID: "p-binh", Nickname: "Bình", NicknameKey: "bình", Token: strings.Repeat("b", 64),
			Budget: 1_000_000, TotalValue: 0,
			JoinedAt: resetNow.Add(time.Second).UnixMilli(), LastSeenAt: resetNow.Add(time.Second).UnixMilli(),
		},
	}
	items := []ItemRow{
		{
			ID: "it-1", OrderIndex: 0, Name: "Bức tranh", Description: "Sơn dầu",
			AuctionType: "english", TruthValue: 350_000,
			HintMin: int64Ptr(200_000), HintMax: int64Ptr(500_000),
			StartPrice: 100_000, MinIncrement: int64Ptr(10_000), DurationMS: int64Ptr(30_000),
			AntiSnipeMode: "extend", AntiSnipeWindowMS: int64Ptr(10_000), MaxExtensions: int64Ptr(2),
			Status: "settled",
		},
		{
			ID: "it-2", OrderIndex: 1, Name: "Đồng hồ", AuctionType: "english",
			TruthValue: 500_000, StartPrice: 100_000, MinIncrement: int64Ptr(10_000),
			DurationMS: int64Ptr(30_000), AntiSnipeMode: "disabled", Status: "pending",
		},
	}
	startedAt := resetNow.UnixMilli()
	deadlineAt := resetNow.Add(30 * time.Second).UnixMilli()
	settledAt := resetNow.Add(5 * time.Second).UnixMilli()

	if err := st.Commit(Mutation{
		Phase: "SETTLED", CurrentItemID: "it-1", Revision: 5, UpdatedAt: settledAt,
		Players: players,
		Items:   items,
		Runtime: &RuntimeRow{
			ItemID: "it-1", StartedAt: startedAt, DeadlineAt: &deadlineAt,
			SettledAt: &settledAt, Revision: 5, ExtensionsUsed: 0,
		},
		Bids: []BidRow{
			{ItemID: "it-1", PlayerID: "p-an", Amount: 100_000, Kind: "bid", CreatedAt: startedAt},
			{ItemID: "it-1", PlayerID: "p-binh", Amount: 110_000, Kind: "bid", CreatedAt: settledAt},
		},
	}); err != nil {
		t.Fatalf("seeding the played round: %v", err)
	}

	// Phase 6 owns the sealed-bid submit path; the row is written here the way
	// that path will leave it, so the reset has a real child row to remove.
	const insertSealed = `
INSERT INTO sealed_bids (item_id, player_id, amount, submitted_at, updated_at)
VALUES ($1, $2, $3, $4, $5)`
	if _, err := st.DB.Exec(insertSealed, "it-1", "p-an", 250_000, startedAt, startedAt); err != nil {
		t.Fatalf("seeding a sealed bid: %v", err)
	}
	return players
}

// TestResetWriteSet is D-78's store contract: the reset write set clears the
// bid trail, the sealed bids and the runtime rows, deletes the players only
// after its children are gone (T-04-05), keeps the item configuration, and
// leaves a file a reopened process observes identically.
//
// The negative control is the point of the ordering: the SAME handle, with
// foreign keys enforced, refuses to drop a player while a bid still references
// it — so the assertion cannot pass by accident on a build where SQLite silently
// ignores the constraint.
func TestResetWriteSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")

	st, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := st.Bootstrap(resetNow); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	seeded := seedPlayedRound(t, st)

	// The fixture must be a real played round, or the reset asserts nothing.
	for _, tc := range []struct {
		table string
		want  int
	}{{"players", 2}, {"items", 2}, {"bids", 2}, {"sealed_bids", 1}, {"auction_runtime", 1}} {
		if got := tableRows(t, st, tc.table); got != tc.want {
			t.Fatalf("fixture %s rows = %d, want %d", tc.table, got, tc.want)
		}
	}

	// PostgreSQL enforces the FK constraints on every connection; the negative
	// delete control below proves the child-before-parent order is load-bearing.

	// --- keep_players: the trail and the timing go, the identities stay -------
	restored := []PlayerRow{
		{
			ID: "p-an", Nickname: "An", NicknameKey: "an", Token: seeded[0].Token,
			Budget: 1_000_000, TotalValue: 0,
			JoinedAt: seeded[0].JoinedAt, LastSeenAt: seeded[0].LastSeenAt,
		},
		{
			ID: "p-binh", Nickname: "Bình", NicknameKey: "bình", Token: seeded[1].Token,
			Budget: 1_000_000, TotalValue: 0,
			JoinedAt: seeded[1].JoinedAt, LastSeenAt: seeded[1].LastSeenAt,
		},
	}
	if err := st.Commit(Mutation{
		Phase: "LOBBY", Revision: 6, UpdatedAt: resetNow.UnixMilli(),
		Players:         restored,
		ClearBids:       true,
		ClearSealedBids: true,
		DeleteRuntime:   true,
	}); err != nil {
		t.Fatalf("keep_players reset: %v", err)
	}

	for _, tc := range []struct {
		table string
		want  int
	}{{"bids", 0}, {"sealed_bids", 0}, {"auction_runtime", 0}, {"players", 2}, {"items", 2}} {
		if got := tableRows(t, st, tc.table); got != tc.want {
			t.Errorf("%s rows after the keep_players reset = %d, want %d", tc.table, got, tc.want)
		}
	}

	game, err := st.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame after the reset: %v", err)
	}
	if game.Phase != "LOBBY" {
		t.Errorf("phase after the reset = %q, want LOBBY", game.Phase)
	}
	if game.CurrentItemID != "" {
		t.Errorf("current_item_id after the reset = %q, want empty", game.CurrentItemID)
	}
	if game.Revision != 6 {
		t.Errorf("revision after the reset = %d, want 6", game.Revision)
	}

	rows, err := st.LoadPlayers()
	if err != nil {
		t.Fatalf("LoadPlayers after the reset: %v", err)
	}
	for i, want := range restored {
		if i >= len(rows) {
			t.Fatalf("LoadPlayers returned %d rows, want %d", len(rows), len(restored))
		}
		if rows[i] != want {
			t.Errorf("player %d after the reset = %+v,\nwant the restored row %+v", i, rows[i], want)
		}
	}

	// The item configuration is untouched: only the status moved, and the
	// keep_players reset does not even move that at the store's level.
	items, err := st.LoadItems()
	if err != nil {
		t.Fatalf("LoadItems after the reset: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("LoadItems returned %d rows after the reset, want 2", len(items))
	}
	if items[0].Name != "Bức tranh" || items[0].TruthValue != 350_000 || items[0].Status != "settled" {
		t.Errorf("item 0 after the reset = %+v, want the untouched fixture row", items[0])
	}
	if items[1].Name != "Đồng hồ" || items[1].TruthValue != 500_000 || items[1].Status != "pending" {
		t.Errorf("item 1 after the reset = %+v, want the untouched fixture row", items[1])
	}

	// --- the negative control: children before parents -------------------------
	//
	// One bid is written back so a player row has a dependent again, and the
	// player delete is then issued WITHOUT the child clears. The handle enforces
	// foreign keys, so this is refused — which is exactly why Commit's order is
	// clear → clear → delete and not the other way round.
	if _, err := st.DB.Exec(
		`INSERT INTO bids (item_id, player_id, amount, kind, created_at) VALUES ($1, $2, $3, $4, $5)`,
		"it-1", "p-an", 120_000, "bid", resetNow.UnixMilli(),
	); err != nil {
		t.Fatalf("re-seeding a bid for the negative control: %v", err)
	}
	err = st.Commit(Mutation{
		Phase: "LOBBY", Revision: 7, UpdatedAt: resetNow.UnixMilli(),
		DeletedPlayers: []string{"p-an"},
	})
	if err == nil {
		t.Fatal("deleting a player while a bid still references it was accepted; the child-before-parent order is not load-bearing")
	}
	if got := tableRows(t, st, "players"); got != 2 {
		t.Errorf("players rows after the refused delete = %d, want the unchanged 2", got)
	}

	// --- clear_players: the same reset, plus every identity --------------------
	if err := st.Commit(Mutation{
		Phase: "LOBBY", Revision: 8, UpdatedAt: resetNow.UnixMilli(),
		ClearBids:       true,
		ClearSealedBids: true,
		DeleteRuntime:   true,
		DeletedPlayers:  []string{"p-an", "p-binh"},
	}); err != nil {
		t.Fatalf("clear_players reset: %v", err)
	}

	for _, tc := range []struct {
		table string
		want  int
	}{{"bids", 0}, {"sealed_bids", 0}, {"auction_runtime", 0}, {"players", 0}, {"items", 2}} {
		if got := tableRows(t, st, tc.table); got != tc.want {
			t.Errorf("%s rows after the clear_players reset = %d, want %d", tc.table, got, tc.want)
		}
	}

	// --- the reopened file observes the same reset ----------------------------
	if err := st.Close(); err != nil {
		t.Fatalf("closing the first handle: %v", err)
	}
	reopened, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("reopening %q: %v", path, err)
	}
	defer func() { _ = reopened.Close() }()

	for _, tc := range []struct {
		table string
		want  int
	}{{"bids", 0}, {"sealed_bids", 0}, {"auction_runtime", 0}, {"players", 0}, {"items", 2}} {
		if got := tableRows(t, reopened, tc.table); got != tc.want {
			t.Errorf("%s rows after the reopen = %d, want %d", tc.table, got, tc.want)
		}
	}
	game, err = reopened.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame after the reopen: %v", err)
	}
	if game.Phase != "LOBBY" || game.Revision != 8 || game.CurrentItemID != "" {
		t.Errorf("game row after the reopen = %+v, want LOBBY / revision 8 / no current item", game)
	}
	items, err = reopened.LoadItems()
	if err != nil {
		t.Fatalf("LoadItems after the reopen: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("items after the reopen = %d rows, want the 2 configured items", len(items))
	}
}
