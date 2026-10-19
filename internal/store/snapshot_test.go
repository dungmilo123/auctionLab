package store

import (
	"auctionlab/internal/testpg"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestRestartRoundTrip is criterion 5's persistence half: a mutation is written
// in one transaction and a reopened database returns exactly what was written —
// players, budgets, the phase and the one canonical game revision.
func TestRestartRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	now := time.UnixMilli(1_700_000_000_000)

	st, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := st.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Literals, not values derived from the implementation: the round trip is
	// only meaningful if the expectation was written down before the write.
	players := []PlayerRow{
		{
			ID: "p-an", Nickname: "An", NicknameKey: "an",
			Token:  strings.Repeat("a", 64),
			Budget: 1_000_000, TotalValue: 0,
			JoinedAt: now.UnixMilli(), LastSeenAt: now.UnixMilli(),
		},
		{
			ID: "p-binh", Nickname: "Bình", NicknameKey: "bình",
			Token:  strings.Repeat("b", 64),
			Budget: 750_000, TotalValue: 250_000,
			JoinedAt: now.Add(time.Second).UnixMilli(), LastSeenAt: now.Add(2 * time.Second).UnixMilli(),
		},
	}

	if err := st.Commit(Mutation{
		Phase:     "LOBBY",
		Revision:  2,
		UpdatedAt: now.UnixMilli(),
		Players:   players,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("closing the first handle: %v", err)
	}

	reopened, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("reopening %q: %v", path, err)
	}
	defer func() { _ = reopened.Close() }()

	game, err := reopened.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame after restart: %v", err)
	}
	if game.Phase != "LOBBY" {
		t.Errorf("phase after restart = %q, want %q", game.Phase, "LOBBY")
	}
	if game.Revision != 2 {
		t.Errorf("revision after restart = %d, want 2: game.revision is the one canonical counter (D-35)", game.Revision)
	}

	got, err := reopened.LoadPlayers()
	if err != nil {
		t.Fatalf("LoadPlayers after restart: %v", err)
	}
	if !reflect.DeepEqual(got, players) {
		t.Errorf("players after restart = %+v,\nwant the rows written before the restart %+v", got, players)
	}
}

// TestCommitStressHasNoBusyErrors is research Pitfall 10's acceptance check: the
// single-connection pool serializes concurrent writers, and the DSN's
// busy_timeout is the second line of defence. A SQLITE_BUSY here would be a
// process exit under D-34's fail-hard policy.
func TestCommitStressHasNoBusyErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")

	st, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := st.Bootstrap(time.UnixMilli(1_700_000_000_000)); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	rows := []PlayerRow{{
		ID: "p-1", Nickname: "An", NicknameKey: "an", Token: strings.Repeat("c", 64),
		Budget: 1_000_000, JoinedAt: 1_700_000_000_000, LastSeenAt: 1_700_000_000_000,
	}}

	const workers = 20
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := st.Commit(Mutation{
				Phase:     "LOBBY",
				Revision:  int64(i + 1),
				UpdatedAt: int64(i + 1),
				Players:   rows,
			}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent commit failed: %v", err)
		if strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked") {
			t.Errorf("concurrent writers hit %v; SetMaxOpenConns(1) must serialize them", err)
		}
	}

	game, err := st.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if game.Revision < 1 || game.Revision > workers {
		t.Errorf("revision = %d after %d commits, want one of the committed values", game.Revision, workers)
	}
}

// int64Ptr is the test's nullable-literal helper: the row types use pointers so
// "absent" (SQL NULL) and a legitimate 0 stay distinguishable in the table.
func int64Ptr(v int64) *int64 { return &v }

// TestAuctionWriteSetRoundTrip is RT-06's storage half: ONE Commit writes the
// game row, players, items, the runtime row and the append-only bid rows in a
// single transaction, and a reopened database returns exactly what was written.
// The rows are literals, not values derived from the implementation, because
// the round trip is only meaningful if the expectation was written down first.
func TestAuctionWriteSetRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	now := time.UnixMilli(1_700_000_000_000)
	startedAt := now.Add(5 * time.Second).UnixMilli()
	deadlineAt := startedAt + 30_000

	st, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := st.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	players := []PlayerRow{{
		ID: "p-an", Nickname: "An", NicknameKey: "an", Token: strings.Repeat("a", 64),
		Budget: 1_000_000, TotalValue: 0, JoinedAt: now.UnixMilli(), LastSeenAt: now.UnixMilli(),
	}}
	items := []ItemRow{
		{
			ID: "it-1", OrderIndex: 0, Name: "Bức tranh", Description: "Sơn dầu",
			ImageURL: "https://example.invalid/it-1.png", AuctionType: "english",
			TruthValue: 350_000, HintMin: int64Ptr(200_000), HintMax: int64Ptr(500_000),
			StartPrice: 100_000, MinIncrement: int64Ptr(10_000), DurationMS: int64Ptr(30_000),
			AntiSnipeMode: "extend", AntiSnipeWindowMS: int64Ptr(10_000),
			MaxExtensions: int64Ptr(2), Status: "live",
		},
		{
			ID: "it-2", OrderIndex: 1, Name: "Đồng hồ", AuctionType: "dutch",
			TruthValue: 0, StartPrice: 50_000, FloorPrice: int64Ptr(20_000),
			DecrementStep: int64Ptr(5_000), DecrementIntervalMS: int64Ptr(2_000),
			Status: "pending",
		},
	}
	runtime := &RuntimeRow{
		ItemID: "it-1", StartedAt: startedAt, DeadlineAt: int64Ptr(deadlineAt),
		Revision: 4, ExtensionsUsed: 1,
	}
	bids := []BidRow{
		{ItemID: "it-1", PlayerID: "p-an", Amount: 100_000, Kind: "bid", CreatedAt: startedAt + 1_000},
		{ItemID: "it-1", PlayerID: "p-an", Amount: 110_000, Kind: "bid", CreatedAt: startedAt + 2_000},
	}

	if err := st.Commit(Mutation{
		Phase: "LIVE", CurrentItemID: "it-1", Revision: 4, UpdatedAt: now.UnixMilli(),
		Players: players, Items: items, Runtime: runtime, Bids: bids,
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("closing the first handle: %v", err)
	}

	reopened, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("reopening %q: %v", path, err)
	}
	defer func() { _ = reopened.Close() }()

	game, err := reopened.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame after restart: %v", err)
	}
	if game.Phase != "LIVE" || game.CurrentItemID != "it-1" || game.Revision != 4 {
		t.Errorf("game row after restart = %+v, want LIVE/it-1/rev 4", game)
	}

	gotItems, err := reopened.LoadItems()
	if err != nil {
		t.Fatalf("LoadItems after restart: %v", err)
	}
	if !reflect.DeepEqual(gotItems, items) {
		t.Errorf("items after restart = %+v,\nwant %+v", gotItems, items)
	}

	gotRuntime, ok, err := reopened.LoadRuntime("it-1")
	if err != nil {
		t.Fatalf("LoadRuntime after restart: %v", err)
	}
	if !ok {
		t.Fatal("LoadRuntime reported no runtime row for it-1 after the restart")
	}
	if !reflect.DeepEqual(gotRuntime, *runtime) {
		t.Errorf("runtime after restart = %+v,\nwant %+v", gotRuntime, *runtime)
	}

	gotBids, err := reopened.LoadBids()
	if err != nil {
		t.Fatalf("LoadBids after restart: %v", err)
	}
	wantBids := []BidRow{
		{ID: 1, ItemID: "it-1", PlayerID: "p-an", Amount: 100_000, Kind: "bid", CreatedAt: startedAt + 1_000},
		{ID: 2, ItemID: "it-1", PlayerID: "p-an", Amount: 110_000, Kind: "bid", CreatedAt: startedAt + 2_000},
	}
	if !reflect.DeepEqual(gotBids, wantBids) {
		t.Errorf("bids after restart = %+v,\nwant the append-only rows %+v", gotBids, wantBids)
	}

	t.Run("bids append across commits and the item delete is durable", func(t *testing.T) {
		if err := reopened.Commit(Mutation{
			Phase: "LIVE", CurrentItemID: "it-1", Revision: 5, UpdatedAt: now.UnixMilli(),
			DeletedItems: []string{"it-2"},
			Bids:         []BidRow{{ItemID: "it-1", PlayerID: "p-an", Amount: 120_000, Kind: "bid", CreatedAt: startedAt + 3_000}},
		}); err != nil {
			t.Fatalf("second Commit: %v", err)
		}

		items, err := reopened.LoadItems()
		if err != nil {
			t.Fatalf("LoadItems: %v", err)
		}
		if len(items) != 1 || items[0].ID != "it-1" {
			t.Errorf("items after the delete = %+v, want only it-1", items)
		}
		bids, err := reopened.LoadBids()
		if err != nil {
			t.Fatalf("LoadBids: %v", err)
		}
		if len(bids) != 3 || bids[2].Amount != 120_000 {
			t.Errorf("bids after the append = %+v, want three rows ending at 120000", bids)
		}
	})
}

// TestSealedBidsUpsertPreservesFirstSubmission is D-108/SEAL-05's storage half:
// `sealed_bids` keeps ONE row per (item, player), an edit rewrites the amount
// and `updated_at`, and `submitted_at` — the tie-break instant — never moves.
// The sealed write rides the same transaction as `game.revision` (D-110).
func TestSealedBidsUpsertPreservesFirstSubmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	now := time.UnixMilli(1_700_000_000_000)

	st, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := st.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	it := ItemRow{
		ID: "it-s1", OrderIndex: 0, Name: "Tranh", AuctionType: "sealed_first",
		TruthValue: 350_000, StartPrice: 100_000, DurationMS: int64Ptr(30_000),
		AntiSnipeMode: "disabled", Status: "live",
	}
	an := PlayerRow{
		ID: "p-an", Nickname: "An", NicknameKey: "an", Token: strings.Repeat("a", 64),
		Budget: 1_000_000, JoinedAt: now.UnixMilli(), LastSeenAt: now.UnixMilli(),
	}
	startedAt := now.UnixMilli()

	first := startedAt + 1_000
	if err := st.Commit(Mutation{
		Phase: "LIVE", CurrentItemID: it.ID, Revision: 2, UpdatedAt: startedAt,
		Players: []PlayerRow{an}, Items: []ItemRow{it},
		Runtime: &RuntimeRow{ItemID: it.ID, StartedAt: startedAt, DeadlineAt: int64Ptr(startedAt + 30_000), Revision: 2},
		SealedBids: []SealedBidRow{{
			ItemID: it.ID, PlayerID: an.ID, Amount: 150_000, SubmittedAt: first, UpdatedAt: first,
		}},
	}); err != nil {
		t.Fatalf("the first sealed commit: %v", err)
	}

	// The edit carries the WRONG submitted_at on purpose: the caller hands the
	// edit's own instant, exactly as a naive submit path would, so a pass can
	// only come from the statement's omission of that column — never from the
	// Go side quietly re-sending the old value.
	edited := first + 4_000
	if err := st.Commit(Mutation{
		Phase: "LIVE", CurrentItemID: it.ID, Revision: 3, UpdatedAt: edited,
		SealedBids: []SealedBidRow{{
			ItemID: it.ID, PlayerID: an.ID, Amount: 180_000, SubmittedAt: edited, UpdatedAt: edited,
		}},
	}); err != nil {
		t.Fatalf("the edit commit: %v", err)
	}

	rows, err := st.LoadSealedBids()
	if err != nil {
		t.Fatalf("LoadSealedBids: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("sealed_bids holds %d row(s) after an edit, want exactly 1 (D-108): %+v", len(rows), rows)
	}
	if rows[0].Amount != 180_000 {
		t.Errorf("amount = %d after the edit, want the last one 180000 (SEAL-02)", rows[0].Amount)
	}
	if rows[0].SubmittedAt != first {
		t.Errorf("submitted_at = %d after an edit, want the FIRST submission's %d — the tie-break instant never moves (D-108/SEAL-05)", rows[0].SubmittedAt, first)
	}
	if rows[0].UpdatedAt != edited {
		t.Errorf("updated_at = %d after the edit, want %d", rows[0].UpdatedAt, edited)
	}

	// D-110: the sealed write is part of the mutation, not a second one.
	game, err := st.LoadGame()
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if game.Revision != 3 {
		t.Errorf("game revision = %d after the sealed write, want the committed 3 — the row rides the mutation's transaction (D-110)", game.Revision)
	}

	// A second key is a second row: the upsert is per (item, player), never a
	// whole-table replace.
	binh := PlayerRow{
		ID: "p-binh", Nickname: "Binh", NicknameKey: "binh", Token: strings.Repeat("b", 64),
		Budget: 1_000_000, JoinedAt: startedAt, LastSeenAt: startedAt,
	}
	if err := st.Commit(Mutation{
		Phase: "LIVE", CurrentItemID: it.ID, Revision: 4, UpdatedAt: edited,
		Players: []PlayerRow{binh},
		SealedBids: []SealedBidRow{{
			ItemID: it.ID, PlayerID: binh.ID, Amount: 200_000, SubmittedAt: edited, UpdatedAt: edited,
		}},
	}); err != nil {
		t.Fatalf("the second player's commit: %v", err)
	}
	rows, err = st.LoadSealedBids()
	if err != nil {
		t.Fatalf("LoadSealedBids after the second player: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("sealed_bids holds %d row(s) for two players, want 2: %+v", len(rows), rows)
	}
	// The loader's order is total: item_id, then submitted_at, then player id.
	if rows[0].PlayerID != an.ID || rows[1].PlayerID != binh.ID {
		t.Errorf("loaded order = %s,%s, want the first submission first (total order)", rows[0].PlayerID, rows[1].PlayerID)
	}
}

// TestRuntimeRoundTrip pins the nullable half of the runtime row: a row that
// never settled keeps settled_at NULL (not 0), and an item without a runtime
// row reports "no row" instead of a zero-valued one.
func TestRuntimeRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	now := time.UnixMilli(1_700_000_000_000)

	st, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(); err != nil {
		t.Fatalf("Schema: %v", err)
	}
	if err := st.Bootstrap(now); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if err := st.Commit(Mutation{
		Phase: "LIVE", Revision: 1, UpdatedAt: now.UnixMilli(),
		Items:   []ItemRow{{ID: "it-1", Name: "x", AuctionType: "english", StartPrice: 1, Status: "live"}},
		Runtime: &RuntimeRow{ItemID: "it-1", StartedAt: 111, Revision: 1},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, ok, err := st.LoadRuntime("it-1")
	if err != nil {
		t.Fatalf("LoadRuntime: %v", err)
	}
	if !ok {
		t.Fatal("LoadRuntime reported no row for a committed runtime row")
	}
	if got.DeadlineAt != nil {
		t.Errorf("deadline_at = %v, want NULL (nil) for an item with no deadline", *got.DeadlineAt)
	}
	if got.SettledAt != nil {
		t.Errorf("settled_at = %v, want NULL (nil) before settlement", *got.SettledAt)
	}
	if got.ExtensionsUsed != 0 {
		t.Errorf("extensions_used = %d, want the default 0", got.ExtensionsUsed)
	}

	if _, ok, err := st.LoadRuntime("it-absent"); err != nil {
		t.Fatalf("LoadRuntime for an unknown item: %v", err)
	} else if ok {
		t.Error("LoadRuntime reported a row for an item that has never run")
	}
}

// TestLoadGameWithoutBootstrapIsAnError pins the boot order: Bootstrap runs
// before the first LoadGame, so a missing row must fail loudly instead of
// handing back a zero-valued game row that looks like LOBBY revision 0.
func TestLoadGameWithoutBootstrapIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auction.db")
	st, err := Open(testpg.URL(t, path))
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(); err != nil {
		t.Fatalf("Schema: %v", err)
	}

	_, err = st.LoadGame()
	if err == nil {
		t.Fatal("LoadGame returned a row for a database that was never bootstrapped")
	}
	if !errors.Is(err, sql.ErrNoRows) && !strings.Contains(err.Error(), "game row") {
		t.Errorf("LoadGame error = %v, want it to say the game row is missing", err)
	}
}
