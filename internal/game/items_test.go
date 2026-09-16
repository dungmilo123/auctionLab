package game

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// This file is ADMN-01/ADMN-02's contract: the item input validation (D-66), the
// delete/reorder guards, and the admin command semantics (D-65) — including the
// one widened phase-machine edge (FINALE → ITEM_INTRO) that D-65's `item.start`
// requires.

// opt returns a pointer to a literal: the ItemInput's nullable numerics.
func opt(v int64) *int64 { return &v }

// validEnglishInput is one complete D-66 English item. Each case below mutates
// exactly one field, so a rejection names the field it is about.
func validEnglishInput() ItemInput {
	return ItemInput{
		Name: "Bức tranh", Description: "Sơn dầu", AuctionType: "english",
		TruthValue: 350_000,
		HintMin:    opt(200_000), HintMax: opt(500_000),
		StartPrice: 100_000,
		MinIncrement: opt(10_000), DurationMS: opt(30_000),
		AntiSnipeMode: "extend", AntiSnipeWindowMS: opt(10_000), MaxExtensions: opt(2),
	}
}

// itemIDs returns the engine's presentation order.
func (env *testEnv) itemIDs(t *testing.T) []string {
	t.Helper()
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	return append([]string(nil), env.engine.state.itemOrder...)
}

// itemOf reads one item row under the engine lock.
func (env *testEnv) item(t *testing.T, itemID string) *item {
	t.Helper()
	env.engine.mu.RLock()
	defer env.engine.mu.RUnlock()
	it := env.engine.state.items[itemID]
	if it == nil {
		t.Fatalf("no item %q in the engine", itemID)
	}
	return it
}

// mustUpsertItem creates one item and returns its server-minted id.
func mustUpsertItem(t *testing.T, env *testEnv, in ItemInput) string {
	t.Helper()
	before := env.itemIDs(t)
	if err := env.engine.ItemUpsert(in); err != nil {
		t.Fatalf("ItemUpsert(%q): %v", in.Name, err)
	}
	after := env.itemIDs(t)
	if len(after) != len(before)+1 {
		t.Fatalf("ItemUpsert appended %d item(s), want 1 (%v -> %v)", len(after)-len(before), before, after)
	}
	return after[len(after)-1]
}

// TestItemUpsertValidation is D-66/H15: one ordered validator, rune-counted
// names, bounded integers, the per-type required fields — and a refusal that
// leaves the state exactly as it was.
func TestItemUpsertValidation(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	revBefore := env.engine.Rev()

	cases := []struct {
		name    string
		mutate  func(in *ItemInput)
		wantErr string
	}{
		{"an empty name is rejected", func(in *ItemInput) { in.Name = "   " }, "name"},
		{"an 81-rune name is rejected", func(in *ItemInput) { in.Name = strings.Repeat("ế", 81) }, "name"},
		{"an unknown auction type is rejected", func(in *ItemInput) { in.AuctionType = "english " }, "auction_type"},
		{"a negative truth value is rejected", func(in *ItemInput) { in.TruthValue = -1 }, "truth_value"},
		{"a truth value above the bound is rejected", func(in *ItemInput) { in.TruthValue = 1_000_000_000_001 }, "truth_value"},
		{"hint_min above hint_max is rejected", func(in *ItemInput) { in.HintMin, in.HintMax = opt(500_000), opt(100_000) }, "hint"},
		{"a start price above the bound is rejected", func(in *ItemInput) { in.StartPrice = 1_000_000_000_001 }, "start_price"},
		{"an English item without min_increment is rejected", func(in *ItemInput) { in.MinIncrement = nil }, "min_increment"},
		{"an English item with a zero min_increment is rejected", func(in *ItemInput) { in.MinIncrement = opt(0) }, "min_increment"},
		{"an English item without duration_ms is rejected", func(in *ItemInput) { in.DurationMS = nil }, "duration_ms"},
		{"an unknown anti-snipe mode is rejected", func(in *ItemInput) { in.AntiSnipeMode = "always" }, "anti_snipe_mode"},
		{"extend without a window is rejected", func(in *ItemInput) { in.AntiSnipeWindowMS = nil }, "anti_snipe_window_ms"},
		{"a zero anti-snipe window is rejected", func(in *ItemInput) { in.AntiSnipeWindowMS = opt(0) }, "anti_snipe_window_ms"},
		{"a zero max_extensions is rejected", func(in *ItemInput) { in.MaxExtensions = opt(0) }, "max_extensions"},
		{"a Dutch item with a zero decrement step is rejected", func(in *ItemInput) {
			in.AuctionType = "dutch"
			in.DecrementStep = opt(0)
		}, "decrement_step"},
		{"a Dutch floor price above the bound is rejected", func(in *ItemInput) {
			in.AuctionType = "dutch"
			in.FloorPrice = opt(1_000_000_000_001)
		}, "floor_price"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validEnglishInput()
			tc.mutate(&in)

			err := env.engine.ItemUpsert(in)
			if err == nil {
				t.Fatalf("ItemUpsert accepted %+v; want a rejection", in)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name %q, so the presenter cannot act on it", err, tc.wantErr)
			}
			if got := len(env.itemIDs(t)); got != 0 {
				t.Errorf("%d item(s) exist after a rejected input, want 0: a rejection never mutates", got)
			}
			if got := env.engine.Rev(); got != revBefore {
				t.Errorf("revision = %d after a rejected input, want %d", got, revBefore)
			}
		})
	}

	t.Run("a valid English input creates a pending item at the end of the list", func(t *testing.T) {
		rev := env.engine.Rev()
		id := mustUpsertItem(t, env, validEnglishInput())

		if len(id) != 32 {
			t.Errorf("minted item id %q has %d characters, want 32 (16 crypto/rand bytes as hex)", id, len(id))
		}
		it := env.item(t, id)
		if it.status != "pending" {
			t.Errorf("new item status = %q, want pending", it.status)
		}
		if it.orderIndex != 0 {
			t.Errorf("first item order_index = %d, want 0", it.orderIndex)
		}
		if got := env.engine.Rev(); got != rev+1 {
			t.Errorf("revision = %d across a create, want %d", got, rev+1)
		}
		events := env.lastEvents()
		if len(events) != 1 || events[0].Type != EventSnapshot {
			t.Errorf("item CRUD emitted %+v, want a single snapshot event (D-66)", events)
		}

		rows, err := env.store.LoadItems()
		if err != nil {
			t.Fatalf("LoadItems: %v", err)
		}
		if len(rows) != 1 || rows[0].ID != id || rows[0].Name != "Bức tranh" || rows[0].Status != "pending" {
			t.Errorf("persisted items = %+v, want the created row", rows)
		}
	})

	t.Run("the name cap counts runes, not bytes", func(t *testing.T) {
		in := validEnglishInput()
		in.Name = strings.Repeat("ế", 80) // 160 bytes, 80 runes
		if err := env.engine.ItemUpsert(in); err != nil {
			t.Errorf("an 80-rune name was rejected: %v (the cap must count runes)", err)
		}
	})

	t.Run("an update by id rewrites the row in place", func(t *testing.T) {
		id := mustUpsertItem(t, env, validEnglishInput())
		orderBefore := env.itemIDs(t)

		in := validEnglishInput()
		in.ID = id
		in.Name = "Tranh mới"
		in.TruthValue = 123_456
		if err := env.engine.ItemUpsert(in); err != nil {
			t.Fatalf("updating %q: %v", id, err)
		}

		it := env.item(t, id)
		if it.name != "Tranh mới" || it.truthValue != 123_456 {
			t.Errorf("updated item = %+v, want the new name and truth value", it)
		}
		if got := env.itemIDs(t); !slices.Equal(got, orderBefore) {
			t.Errorf("order changed across an update: %v -> %v", orderBefore, got)
		}
	})

	t.Run("an unknown id is rejected", func(t *testing.T) {
		in := validEnglishInput()
		in.ID = "it-does-not-exist"
		if err := env.engine.ItemUpsert(in); err == nil {
			t.Fatal("ItemUpsert accepted an unknown id: a client-chosen primary key must not create rows (H14)")
		}
	})

	t.Run("a Dutch item without its mechanics fields is still a valid pending row", func(t *testing.T) {
		in := validEnglishInput()
		in.AuctionType = "dutch"
		in.MinIncrement, in.DurationMS, in.AntiSnipeMode = nil, nil, ""
		in.AntiSnipeWindowMS, in.MaxExtensions = nil, nil
		if err := env.engine.ItemUpsert(in); err != nil {
			t.Errorf("a Dutch item with no optional mechanics was rejected: %v (Phase 5 owns its flow)", err)
		}
	})
}

// TestItemDeleteGuards is D-66's history rule: a pending item can be removed,
// the current item cannot (the machine would be left inconsistent), and a live
// or settled row can never be rewritten or deleted.
func TestItemDeleteGuards(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	var current string

	t.Run("a pending item is deleted and the delete is durable", func(t *testing.T) {
		id := mustUpsertItem(t, env, validEnglishInput())
		if err := env.engine.DeleteItem(id); err != nil {
			t.Fatalf("DeleteItem(%q): %v", id, err)
		}
		if got := env.itemIDs(t); slices.Contains(got, id) {
			t.Errorf("item %q is still in the presentation order after a delete", id)
		}
		rows, err := env.store.LoadItems()
		if err != nil {
			t.Fatalf("LoadItems: %v", err)
		}
		for _, row := range rows {
			if row.ID == id {
				t.Errorf("item %q is still in the database after a delete", id)
			}
		}
	})

	t.Run("the current intro item cannot be deleted", func(t *testing.T) {
		current = mustUpsertItem(t, env, validEnglishInput())
		if err := env.engine.StartItem(current); err != nil {
			t.Fatalf("StartItem: %v", err)
		}

		orderBefore := env.itemIDs(t)
		revBefore := env.engine.Rev()
		if err := env.engine.DeleteItem(current); err == nil {
			t.Fatal("the current item was deleted: ITEM_INTRO would be left with no item (boot would then fail hard)")
		}
		if got := env.itemIDs(t); !slices.Equal(got, orderBefore) {
			t.Errorf("order changed across a refused delete: %v -> %v", orderBefore, got)
		}
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d across a refused delete, want %d", got, revBefore)
		}
	})

	t.Run("a live item cannot be deleted or rewritten", func(t *testing.T) {
		// The intro item from the previous case is still current: start it again
		// to go LIVE, then attack the row.
		if err := env.engine.StartItem(current); err != nil {
			t.Fatalf("StartItem (LIVE): %v", err)
		}

		if err := env.engine.DeleteItem(current); err == nil {
			t.Error("a live item was deleted; the settle history would lose its row")
		}
		in := validEnglishInput()
		in.ID = current
		in.Name = "Đổi trộm"
		if err := env.engine.ItemUpsert(in); err == nil {
			t.Error("a live item was rewritten")
		}
		if got := env.item(t, current).name; got == "Đổi trộm" {
			t.Errorf("the live row now says %q — a refused upsert still wrote", got)
		}
	})

	t.Run("a settled item cannot be deleted or rewritten", func(t *testing.T) {
		if err := env.engine.ForceEnd(); err != nil {
			t.Fatalf("ForceEnd: %v", err)
		}

		if err := env.engine.DeleteItem(current); err == nil {
			t.Error("a settled item was deleted; the reveal would lose its row")
		}
		in := validEnglishInput()
		in.ID = current
		if err := env.engine.ItemUpsert(in); err == nil {
			t.Error("a settled item was rewritten")
		}
		if got := env.item(t, current).status; got != "settled" {
			t.Errorf("item status = %q after refused edits, want settled", got)
		}
	})
}

// TestReorderRequiresPermutation is D-66's reorder rule: only a complete unique
// permutation is accepted, and anything else is refused without a partial write.
func TestReorderRequiresPermutation(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	a := mustUpsertItem(t, env, validEnglishInput())
	b := mustUpsertItem(t, env, validEnglishInput())
	c := mustUpsertItem(t, env, validEnglishInput())

	before := env.itemIDs(t)

	cases := []struct {
		name string
		ids  []string
	}{
		{"a duplicate id is rejected", []string{a, b, b}},
		{"a missing id is rejected", []string{a, b}},
		{"an unknown id is rejected", []string{a, b, "it-nope"}},
		{"a list longer than the catalogue is rejected", []string{a, b, c, a}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			revBefore := env.engine.Rev()
			if err := env.engine.ReorderItems(tc.ids); err == nil {
				t.Fatalf("ReorderItems(%v) was accepted; only a complete unique permutation may be (D-66)", tc.ids)
			}
			if got := env.itemIDs(t); !slices.Equal(got, before) {
				t.Errorf("order = %v after a refused reorder, want the untouched %v", got, before)
			}
			rows, err := env.store.LoadItems()
			if err != nil {
				t.Fatalf("LoadItems: %v", err)
			}
			for i, row := range rows {
				if row.ID != before[i] {
					t.Errorf("persisted row %d = %q, want %q: a refused reorder must not partially write", i, row.ID, before[i])
				}
			}
			if got := env.engine.Rev(); got != revBefore {
				t.Errorf("revision = %d across a refused reorder, want %d", got, revBefore)
			}
		})
	}

	t.Run("a complete permutation rewrites order_index and persists", func(t *testing.T) {
		rev := env.engine.Rev()
		if err := env.engine.ReorderItems([]string{c, a, b}); err != nil {
			t.Fatalf("ReorderItems: %v", err)
		}
		if got := env.itemIDs(t); !slices.Equal(got, []string{c, a, b}) {
			t.Errorf("order = %v, want [c a b]", got)
		}
		rows, err := env.store.LoadItems()
		if err != nil {
			t.Fatalf("LoadItems: %v", err)
		}
		for i, want := range []string{c, a, b} {
			if rows[i].ID != want || rows[i].OrderIndex != int64(i) {
				t.Errorf("persisted row %d = %q@%d, want %q@%d", i, rows[i].ID, rows[i].OrderIndex, want, i)
			}
		}
		if got := env.engine.Rev(); got != rev+1 {
			t.Errorf("revision = %d across a reorder, want %d", got, rev+1)
		}
	})
}

// TestAdminCommandSemantics is D-65's command table on the real engine: the
// legal arms of start/next/skip/force-end, every illegal edge refused via
// CanTransition with nothing changed, and the FINALE → ITEM_INTRO edge D-65
// requires.
func TestAdminCommandSemantics(t *testing.T) {
	env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
	first := mustUpsertItem(t, env, validEnglishInput())
	second := mustUpsertItem(t, env, validEnglishInput())

	t.Run("item.start with an unknown id changes nothing", func(t *testing.T) {
		revBefore := env.engine.Rev()
		if err := env.engine.StartItem("it-nope"); err == nil {
			t.Fatal("item.start accepted an unknown id")
		}
		if got := env.engine.Phase(); got != PhaseLobby {
			t.Errorf("phase = %q, want LOBBY", got)
		}
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d across a refused start, want %d", got, revBefore)
		}
	})

	t.Run("item.start in LOBBY opens the intro", func(t *testing.T) {
		if err := env.engine.StartItem(first); err != nil {
			t.Fatalf("StartItem(%q): %v", first, err)
		}
		if got := env.engine.Phase(); got != PhaseItemIntro {
			t.Fatalf("phase = %q, want ITEM_INTRO", got)
		}
		if got := env.item(t, first).status; got != "intro" {
			t.Errorf("item status = %q, want intro", got)
		}
		events := env.lastEvents()
		if len(events) != 3 || events[0].Type != EventPhaseChanged || events[1].Type != EventItemIntro || events[2].Type != EventSnapshot {
			t.Fatalf("item.start emitted %+v, want phase.changed, item.intro, snapshot", events)
		}
		payload, ok := events[0].Data.(PhaseChanged)
		if !ok {
			t.Fatalf("phase.changed payload is %T, want PhaseChanged", events[0].Data)
		}
		if payload.Phase != string(PhaseItemIntro) || payload.Item == nil || payload.Item.ID != first {
			t.Errorf("phase.changed = %+v, want ITEM_INTRO with item %q", payload, first)
		}
		if item, ok := events[1].Data.(*PublicItem); !ok || item.ID != first || item.Status != "intro" {
			t.Errorf("item.intro payload = %#v, want the intro item", events[1].Data)
		}
	})

	t.Run("item.start with a different id while one is in intro is rejected", func(t *testing.T) {
		revBefore := env.engine.Rev()
		if err := env.engine.StartItem(second); err == nil {
			t.Fatal("item.start resurrected a second item during an intro (a stale id must not move the machine)")
		}
		if got := env.item(t, first).status; got != "intro" {
			t.Errorf("the intro item changed to %q across a refused start", got)
		}
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d across a refused start, want %d", got, revBefore)
		}
	})

	t.Run("item.start on the intro item goes LIVE with the configured duration", func(t *testing.T) {
		env.clock.t = testStart.Add(5 * time.Second)
		rev := env.engine.Rev()
		if err := env.engine.StartItem(first); err != nil {
			t.Fatalf("StartItem (LIVE): %v", err)
		}
		if got := env.engine.Phase(); got != PhaseLive {
			t.Fatalf("phase = %q, want LIVE", got)
		}
		it := env.item(t, first)
		if it.status != "live" {
			t.Errorf("item status = %q, want live", it.status)
		}
		run := env.runOf(t)
		wantStarted := testStart.Add(5 * time.Second).UnixMilli()
		wantDeadline := wantStarted + it.durationMS
		if run.startedAt != wantStarted || run.deadlineAt != wantDeadline {
			t.Errorf("run = started %d / deadline %d, want %d / %d", run.startedAt, run.deadlineAt, wantStarted, wantDeadline)
		}
		if run.extensionsUsed != 0 {
			t.Errorf("extensions_used = %d on a fresh run, want 0", run.extensionsUsed)
		}
		events := env.lastEvents()
		if len(events) != 3 || events[0].Type != EventPhaseChanged || events[1].Type != EventItemLive || events[2].Type != EventSnapshot {
			t.Fatalf("going LIVE emitted %+v, want phase.changed, item.live, snapshot", events)
		}
		if got := env.engine.Rev(); got != rev+1 {
			t.Errorf("revision = %d across a live start, want %d", got, rev+1)
		}

		rt, ok, err := env.store.LoadRuntime(first)
		if err != nil || !ok {
			t.Fatalf("the live start did not persist a runtime row (ok=%t, err=%v)", ok, err)
		}
		if rt.DeadlineAt == nil || *rt.DeadlineAt != wantDeadline {
			t.Errorf("persisted deadline_at = %v, want %d", rt.DeadlineAt, wantDeadline)
		}
	})

	t.Run("item.start is rejected while LIVE", func(t *testing.T) {
		if err := env.engine.StartItem(second); err == nil {
			t.Fatal("item.start was accepted during a live auction")
		}
	})

	t.Run("item.next in ITEM_INTRO skips the item; with nothing pending it enters FINALE", func(t *testing.T) {
		if err := env.engine.ForceEnd(); err != nil {
			t.Fatalf("ForceEnd: %v", err)
		}
		// SETTLED → next pending item
		if err := env.engine.NextItem(); err != nil {
			t.Fatalf("NextItem from SETTLED: %v", err)
		}
		if got := env.engine.Phase(); got != PhaseItemIntro {
			t.Fatalf("phase = %q, want ITEM_INTRO", got)
		}
		if got := env.item(t, second).status; got != "intro" {
			t.Errorf("the next item status = %q, want intro", got)
		}

		rev := env.engine.Rev()
		if err := env.engine.NextItem(); err != nil {
			t.Fatalf("NextItem (skip): %v", err)
		}
		if got := env.item(t, second).status; got != "skipped" {
			t.Errorf("the skipped item status = %q, want skipped (admin.item.next IS item.skip)", got)
		}
		if got := env.engine.Phase(); got != PhaseFinale {
			t.Fatalf("phase = %q with no pending item, want FINALE", got)
		}
		if got := env.engine.Rev(); got != rev+1 {
			t.Errorf("revision = %d across one next, want one bump to %d", got, rev+1)
		}
		events := env.lastEvents()
		if len(events) != 3 || events[0].Type != EventPhaseChanged || events[1].Type != EventFinale || events[2].Type != EventSnapshot {
			t.Fatalf("entering FINALE emitted %+v, want phase.changed, finale then snapshot", events)
		}
		payload, ok := events[0].Data.(PhaseChanged)
		if !ok || payload.Phase != string(PhaseFinale) || payload.Item != nil {
			t.Errorf("phase.changed = %#v, want FINALE with a null item (current_item_id cleared)", events[0].Data)
		}
		// D-89: the finale payload rides the reserved catalog frame and stays
		// last-but-one, so the snapshot is still the frame a resync lands on.
		if _, ok := events[1].Data.(FinaleReveal); !ok {
			t.Errorf("the finale payload is %T, want FinaleReveal", events[1].Data)
		}
		// D-25/H10: the projection agrees — no item is current in FINALE.
		doc := decodeObject(t, env.engine.Snapshot(RolePlayer, "p-nobody"))
		if item, ok := doc["item"]; !ok || item != nil {
			t.Errorf("FINALE player snapshot item = %#v (present=%t), want null", item, ok)
		}
	})

	t.Run("a FINALE item.start is legal again (the D-65 widened edge)", func(t *testing.T) {
		third := mustUpsertItem(t, env, validEnglishInput())
		if err := env.engine.StartItem(third); err != nil {
			t.Fatalf("item.start during FINALE: %v — D-65 requires FINALE → ITEM_INTRO", err)
		}
		if got := env.engine.Phase(); got != PhaseItemIntro {
			t.Errorf("phase = %q, want ITEM_INTRO", got)
		}
		if got := env.item(t, third).status; got != "intro" {
			t.Errorf("item status = %q, want intro", got)
		}
	})

	t.Run("force_end outside LIVE is refused with nothing changed", func(t *testing.T) {
		revBefore := env.engine.Rev()
		if err := env.engine.ForceEnd(); err == nil {
			t.Fatal("force_end was accepted outside LIVE")
		}
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d across a refused force-end, want %d", got, revBefore)
		}
		if got := env.engine.Phase(); got != PhaseItemIntro {
			t.Errorf("phase = %q across a refused force-end, want ITEM_INTRO", got)
		}
	})

	t.Run("item.next in LOBBY is not a legal command", func(t *testing.T) {
		env2 := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		if err := env2.engine.NextItem(); err == nil {
			t.Error("item.next was accepted in LOBBY; the first item is opened by item.start")
		}
	})
}

// TestNoDeadlineNeverGoesLive is WR-02: this phase plays English only, and the
// whole phase hangs off a live run that can settle ON THE CLOCK — the ticker's
// settle arm and the boot's resume both require `deadline_at`. A run started
// without one would sit in LIVE forever (`Force end` its only exit) while the
// English ladder still accepted bids on it, so the LIVE edge is refused for any
// row that cannot carry a deadline. The intro and `item.next` stay available:
// the row is on stage and the presenter skips it the documented way.
func TestNoDeadlineNeverGoesLive(t *testing.T) {
	t.Run("a dutch item opens its intro but never goes live", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))

		// The natural draft the shipped editor produces: D-66 hides duration_ms
		// for dutch, so the payload carries null and the row has no window.
		dutch := validEnglishInput()
		dutch.AuctionType = "dutch"
		dutch.MinIncrement, dutch.DurationMS, dutch.AntiSnipeMode = nil, nil, ""
		dutch.AntiSnipeWindowMS, dutch.MaxExtensions = nil, nil
		id := mustUpsertItem(t, env, dutch)

		if err := env.engine.StartItem(id); err != nil {
			t.Fatalf("item.start (LOBBY → ITEM_INTRO): %v", err)
		}
		if got := env.engine.Phase(); got != PhaseItemIntro {
			t.Fatalf("phase = %q, want ITEM_INTRO", got)
		}

		revBefore := env.engine.Rev()
		err := env.engine.StartItem(id)
		if err == nil {
			t.Fatal("a dutch item went LIVE: the run would have no deadline and could never settle on the clock (WR-02)")
		}
		if !strings.Contains(err.Error(), "english") {
			t.Errorf("error %q does not name the type at fault, so the presenter cannot act on it", err)
		}
		if got := env.engine.Phase(); got != PhaseItemIntro {
			t.Errorf("phase = %q after the refused start, want ITEM_INTRO", got)
		}
		if got := env.item(t, id).status; got != "intro" {
			t.Errorf("item status = %q after the refused start, want intro", got)
		}
		if got := env.engine.Rev(); got != revBefore {
			t.Errorf("revision = %d after a refused start, want the unchanged %d", got, revBefore)
		}
		env.engine.mu.RLock()
		run := env.engine.state.currentAuction
		env.engine.mu.RUnlock()
		if run != nil {
			t.Errorf("a refused start created a run: %+v", run)
		}

		// The documented way out is unchanged: next skips the item.
		if err := env.engine.NextItem(); err != nil {
			t.Fatalf("NextItem after the refusal: %v", err)
		}
		if got := env.itemStatus(id); got != "skipped" {
			t.Errorf("status = %q, want skipped (admin.item.next IS item.skip)", got)
		}
		if got := env.engine.Phase(); got != PhaseFinale {
			t.Errorf("phase = %q, want FINALE with no pending item left", got)
		}
	})

	t.Run("an english row with no duration is refused as well", func(t *testing.T) {
		env := newTestEnv(t, filepath.Join(t.TempDir(), "auction.db"))
		id := mustUpsertItem(t, env, validEnglishInput())
		if err := env.engine.StartItem(id); err != nil {
			t.Fatalf("item.start (LOBBY → ITEM_INTRO): %v", err)
		}

		// D-66 refuses to create such a row (english requires duration_ms > 0),
		// so the only way to reach this state is a row loaded from outside the
		// validator — a hand-edited file. Mutate the loaded row directly and pin
		// that the deadline invariant is enforced at the edge that needs it.
		env.engine.mu.Lock()
		env.engine.state.items[id].durationMS = 0
		env.engine.mu.Unlock()

		if err := env.engine.StartItem(id); err == nil {
			t.Fatal("an english row with no deadline went LIVE: settleDueAtBoot and SettleIfDue would both skip it (WR-02)")
		}
		if got := env.engine.Phase(); got != PhaseItemIntro {
			t.Errorf("phase = %q after the refused start, want ITEM_INTRO", got)
		}
	})
}
