package hub

import (
	"auctionlab/internal/testpg"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
	"auctionlab/internal/store"
)

// This file owns the Phase 5 command surface at the transport boundary:
// `dutch.accept` is a player-only command with NO payload (D-95), and every
// refusal travels as the private `bid.rejected` frame with the price the SERVER
// derived at that instant (D-96) — never a number the phone sent.
//
// The cases are scripted exact frame sequences, like auction_test.go: the ORDER
// of one mutation's siblings and the revision they share are part of the
// contract (D-21/D-57).

// The seeded Dutch lot: 0 < floor < start with a positive step and interval
// (D-92), and every mechanic on the row so a client can derive the same price
// the server charges (D-94).
const (
	dutchItemID      = "it-dutch"
	dutchPlayerID    = "p-an"
	dutchPlayerToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	dutchStartPrice  = int64(100_000)
	dutchFloorPrice  = int64(40_000)
	dutchStep        = int64(10_000)
	dutchIntervalMS  = int64(1_000)
	dutchTruthValue  = int64(350_000)
)

// int64Ptr is the nullable-int helper for the seeded store rows.
func int64Ptr(v int64) *int64 { return &v }

// newLiveDutchHub builds a hub whose engine BOOTS onto a file that already
// carries a LIVE Dutch run and one registered player (whose token
// `dutchPlayerToken` resumes it).
//
// A live Dutch row can only be opened through `admin.item.start` once this
// plan's Task 2 lands the per-type guard (D-92), and the wire contracts below
// are this task's subject — so the fixture seeds the durable rows through the
// store (exactly what the engine package's mustOpenLiveRun does for an English
// run) and lets the real boot validate and resume them (D-101).
func newLiveDutchHub(t *testing.T, clock *testClock, startedAt, budget int64) (*Hub, *game.Engine, *store.Store) {
	t.Helper()

	st, err := store.Open(testpg.URL(t, filepath.Join(t.TempDir(), "auction.db")))
	if err != nil {
		t.Fatalf("opening the seeded database: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("applying the schema: %v", err)
	}
	if err := st.Bootstrap(clock.now()); err != nil {
		t.Fatalf("bootstrapping the seeded game row: %v", err)
	}

	it := store.ItemRow{
		ID: dutchItemID, Name: "Đồng hồ", AuctionType: "dutch", TruthValue: dutchTruthValue,
		StartPrice: dutchStartPrice, FloorPrice: int64Ptr(dutchFloorPrice),
		DecrementStep: int64Ptr(dutchStep), DecrementIntervalMS: int64Ptr(dutchIntervalMS),
		Status: "live",
	}
	an := store.PlayerRow{
		ID: dutchPlayerID, Nickname: "An", NicknameKey: "an", Token: dutchPlayerToken,
		Budget: budget, JoinedAt: startedAt, LastSeenAt: startedAt,
	}
	if err := st.Commit(store.Mutation{
		Phase: "LIVE", CurrentItemID: it.ID, Revision: 1, UpdatedAt: startedAt,
		Items:   []store.ItemRow{it},
		Players: []store.PlayerRow{an},
		Runtime: &store.RuntimeRow{ItemID: it.ID, StartedAt: startedAt, Revision: 1},
	}); err != nil {
		t.Fatalf("seeding the live Dutch run: %v", err)
	}

	engine, err := game.New(game.Config{
		Store: st,
		Now:   clock.now,
		Fatal: func(err error) { t.Errorf("engine reported an unrecoverable commit failure: %v", err) },
	})
	if err != nil {
		t.Fatalf("booting on the seeded Dutch file: %v", err)
	}

	h := newHub(engine, testPolicyWithBuffer(64), testAdminKey)
	h.now = clock.now
	engine.SetPublisher(h.Fanout)
	return h, engine, st
}

// dutchAcceptDial dials the seeded player and completes the D-30 token resume.
// A resume is a presence change, not a mutation (D-37), so the handshake pair is
// followed by exactly one fan-out snapshot — the connection is registered at the
// upgrade, and the resume's own fan-out reaches it (H2/D-33).
func dutchAcceptDial(t *testing.T, h *Hub) (*websocket.Conn, frame) {
	t.Helper()

	conn := dialWS(t, h)
	_, snapshot := join(t, conn, `{"type":"join","role":"player","token":"`+dutchPlayerToken+`"}`)
	expectFrames(t, conn, TypeStateSnapshot)
	return conn, snapshot
}

// TestDutchAcceptSettlesOnTheWire is DTCH-02's happy path at the transport
// edge: ONE real player frame settles the item at the price the SERVER derives
// at that instant, the audit row is committed with the settle, and a hostile
// `amount` key in the payload changes nothing (D-95 — the command has no amount
// field, and D-24 says an extra key is ignored; T-05-01's mitigation).
func TestDutchAcceptSettlesOnTheWire(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	clock := &testClock{t: base.Add(3 * time.Second)} // three intervals past the start
	h, engine, st := newLiveDutchHub(t, clock, base.UnixMilli(), 1_000_000)

	conn, snapshot := dutchAcceptDial(t, h)
	budgetBefore := meOf(t, snapshot).Budget

	writeFrame(t, conn, `{"type":"dutch.accept","amount":1}`)
	frames := expectFrames(t, conn, TypeItemSettled, TypeLeaderboard, TypeStateSnapshot)

	// item.settled carries the public item with the reveal nested under `settle`
	// (D-67), so the price the room reads is the settle object's own.
	settled := dataObject(t, frames[0], "settle")
	if got := frameString(t, settled, "outcome"); got != "sold" {
		t.Errorf("item.settled outcome = %q, want sold", got)
	}
	if got := objectInt(t, settled, "price"); got != 70_000 {
		t.Errorf("item.settled price = %d, want the server derivation 70000 — never the payload's 1", got)
	}
	if got := objectInt(t, settled, "truth_value"); got != dutchTruthValue {
		t.Errorf("item.settled truth_value = %d, want %d", got, dutchTruthValue)
	}
	raw, ok := settled["winner"]
	if !ok || string(raw) == "null" {
		t.Fatalf("item.settled winner = %s, want the accepting player", raw)
	}
	var winner struct {
		Nickname string `json:"nickname"`
	}
	if err := json.Unmarshal(raw, &winner); err != nil {
		t.Fatalf("item.settled winner is not a bidder object: %v", err)
	}
	if winner.Nickname != "An" {
		t.Errorf("item.settled winner = %q, want An", winner.Nickname)
	}

	// One settle, one revision: every sibling frame carries the mutation's own
	// revision (D-21), and the charge lands in the settle's snapshot.
	rev := frames[0].Rev
	for i, env := range frames {
		if env.Rev != rev {
			t.Errorf("frame %d (%s) carries rev %d, want the settle's %d", i, env.Type, env.Rev, rev)
		}
	}
	if got := engine.Rev(); got != rev {
		t.Errorf("engine revision = %d, want the settled %d", got, rev)
	}
	me := meOf(t, frames[2])
	if want := budgetBefore - 70_000 + dutchTruthValue; me.Budget != want {
		t.Errorf("winner budget = %d, want %d (budget − price + truth_value)", me.Budget, want)
	}

	// D-98: the accepted price is durable, with the accept kind.
	rows, err := st.LoadBids()
	if err != nil {
		t.Fatalf("loading the bid audit: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("the wire accept appended %d audit row(s), want exactly 1: %+v", len(rows), rows)
	}
	if rows[0].Kind != "accept" || rows[0].Amount != 70_000 {
		t.Errorf("audit row = %+v, want kind accept at the derived 70000", rows[0])
	}
}

// TestDutchAcceptRoleGate pins D-95's role gate at the transport edge: the
// command is player-only, so an admin or screen sender is answered with the
// frozen `bad_message` and never reaches the engine — while a player's accept
// reaches the private-refusal path (D-96) rather than a protocol error.
func TestDutchAcceptRoleGate(t *testing.T) {
	t.Run("an admin command is bad_message", func(t *testing.T) {
		h, _ := newTestHub(t)
		admin := joinRole(t, h, RoleAdmin)

		writeFrame(t, admin, `{"type":"dutch.accept"}`)
		assertErrorCode(t, readFrame(t, admin), CodeBadMessage)
	})

	t.Run("a screen command is bad_message", func(t *testing.T) {
		h, _ := newTestHub(t)
		screen := joinRole(t, h, RoleScreen)

		writeFrame(t, screen, `{"type":"dutch.accept"}`)
		assertErrorCode(t, readFrame(t, screen), CodeBadMessage)
	})

	t.Run("a player reaches the private refusal path, never a protocol error", func(t *testing.T) {
		h, engine := newTestHub(t)
		player, _ := joinPlayer(t, h, "An")

		revBefore := engine.Rev()
		writeFrame(t, player, `{"type":"dutch.accept"}`)
		env := readFrame(t, player)
		if env.Type != TypeBidRejected {
			t.Fatalf("a player's dutch.accept answered %q, want the private %q (D-95/D-96)", env.Type, TypeBidRejected)
		}
		if got := frameString(t, frameData(t, env), "reason"); got != "window_closed" {
			t.Errorf("refusal reason = %q, want window_closed with no run open", got)
		}
		if env.Rev != revBefore {
			t.Errorf("a refusal carries rev %d, want the current %d: a refusal is not a mutation (D-53)", env.Rev, revBefore)
		}
	})
}

// TestDutchAcceptRefusals pins D-96's refusal payload on the wire: the reason,
// the SERVER's derived price as `amount`, the player's own budget, and a null
// `min_next_bid` (Dutch has no ladder) — with nothing persisted and no revision
// movement.
func TestDutchAcceptRefusals(t *testing.T) {
	t.Run("no live run: window_closed, no ladder, nothing persisted", func(t *testing.T) {
		h, engine := newTestHub(t)
		player, _ := joinPlayer(t, h, "An")
		revBefore := engine.Rev()

		writeFrame(t, player, `{"type":"dutch.accept"}`)
		env := readFrame(t, player)
		if env.Type != TypeBidRejected {
			t.Fatalf("a refusal arrived as %q, want %q", env.Type, TypeBidRejected)
		}
		data := frameData(t, env)
		if got := frameString(t, data, "reason"); got != "window_closed" {
			t.Errorf("refusal reason = %q, want window_closed", got)
		}
		if hasKey(data, "min_next_bid") {
			t.Error("a Dutch refusal advertised a ladder minimum: Dutch has no ladder (D-96)")
		}
		if got := objectInt(t, data, "amount"); got != 0 {
			t.Errorf("refusal amount = %d, want 0: with no item and no run the derivation is total (D-91)", got)
		}
		if got := engine.Rev(); got != revBefore {
			t.Errorf("revision = %d after a refusal, want the unchanged %d", got, revBefore)
		}
	})

	t.Run("a price above the budget is exceeds_budget and changes nothing", func(t *testing.T) {
		base := time.UnixMilli(1_700_000_000_000)
		clock := &testClock{t: base.Add(3 * time.Second)}
		h, engine, st := newLiveDutchHub(t, clock, base.UnixMilli(), 50_000)

		conn, snapshot := dutchAcceptDial(t, h)
		if got := meOf(t, snapshot).Budget; got != 50_000 {
			t.Fatalf("seeded budget = %d, want 50000", got)
		}

		revBefore := engine.Rev()
		writeFrame(t, conn, `{"type":"dutch.accept"}`)
		env := readFrame(t, conn)
		if env.Type != TypeBidRejected {
			t.Fatalf("an over-budget accept answered %q, want %q", env.Type, TypeBidRejected)
		}
		data := frameData(t, env)
		if got := frameString(t, data, "reason"); got != "exceeds_budget" {
			t.Errorf("refusal reason = %q, want exceeds_budget", got)
		}
		if got := objectInt(t, data, "amount"); got != 70_000 {
			t.Errorf("refusal amount = %d, want the derived price 70000 — the number the refusal is about", got)
		}
		if got := objectInt(t, data, "budget"); got != 50_000 {
			t.Errorf("refusal budget = %d, want the player's own 50000", got)
		}
		if hasKey(data, "min_next_bid") {
			t.Error("a Dutch refusal advertised a ladder minimum (D-96)")
		}
		if env.Rev != revBefore {
			t.Errorf("the refusal frame carries rev %d, want the current %d", env.Rev, revBefore)
		}
		if got := engine.Rev(); got != revBefore {
			t.Errorf("revision = %d after a refusal, want the unchanged %d", got, revBefore)
		}
		if got := engine.Phase(); got != game.PhaseLive {
			t.Errorf("phase = %q after a refusal, want LIVE: a refusal settles nothing", got)
		}
		rows, err := st.LoadBids()
		if err != nil {
			t.Fatalf("loading the bid audit: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("a refused accept persisted %d audit row(s), want none: %+v", len(rows), rows)
		}
	})
}

// TestDutchTickOnTheWire is DTCH-01's public frame contract on a real socket:
// the seeded LIVE snapshot carries the derived price, the clock anchor and the
// three mechanics (D-94) and no ladder minimum, and a beat sends those same
// numbers at the CURRENT revision — with neither deadline_at nor remaining_ms,
// because a Dutch run has no window at all (D-93/D-58).
func TestDutchTickOnTheWire(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	clock := &testClock{t: base.Add(3 * time.Second)} // three intervals past the start
	h, engine, _ := newLiveDutchHub(t, clock, base.UnixMilli(), 1_000_000)

	conn, snapshot := dutchAcceptDial(t, h)

	// The snapshot a reloading phone renders from: every mechanic plus the
	// anchor, and NOTHING ladder-shaped.
	item := dataObject(t, snapshot, "item")
	if got := objectInt(t, item, "current_price"); got != 70_000 {
		t.Errorf("snapshot current_price = %d, want the derived 70000 at this instant (DTCH-01)", got)
	}
	if got := objectInt(t, item, "started_at"); got != base.UnixMilli() {
		t.Errorf("snapshot started_at = %d, want the run's %d (D-94)", got, base.UnixMilli())
	}
	if got := objectInt(t, item, "floor_price"); got != dutchFloorPrice {
		t.Errorf("snapshot floor_price = %d, want %d", got, dutchFloorPrice)
	}
	if got := objectInt(t, item, "decrement_step"); got != dutchStep {
		t.Errorf("snapshot decrement_step = %d, want %d", got, dutchStep)
	}
	if got := objectInt(t, item, "decrement_interval_ms"); got != dutchIntervalMS {
		t.Errorf("snapshot decrement_interval_ms = %d, want %d", got, dutchIntervalMS)
	}
	if hasKey(item, "min_next_bid") {
		t.Error("a Dutch snapshot advertises min_next_bid: a Dutch price has no ladder (D-98)")
	}
	if hasKey(item, "deadline_at") {
		t.Error("a Dutch snapshot advertises deadline_at: a Dutch run has no deadline (D-93)")
	}
	if hasKey(item, "top_bidder") {
		t.Error("a Dutch snapshot advertises a top bidder: an acceptance is not a ladder bid (D-98)")
	}

	// One beat: the same derived number, the anchor to re-anchor on, no window —
	// and the current revision, because a tick is not a mutation (D-58).
	rev := engine.Rev()
	h.tick()
	beat := readType(t, conn, TypePriceTick)
	if beat.Rev != rev {
		t.Errorf("tick rev = %d, want the current %d (D-58)", beat.Rev, rev)
	}
	data := frameData(t, beat)
	if got := objectInt(t, data, "current_price"); got != 70_000 {
		t.Errorf("tick current_price = %d, want the derived 70000 (DTCH-01)", got)
	}
	if got := objectInt(t, data, "started_at"); got != base.UnixMilli() {
		t.Errorf("tick started_at = %d, want the run's %d so the client re-anchors (D-94)", got, base.UnixMilli())
	}
	if hasKey(data, "deadline_at") {
		t.Error("a Dutch tick carries deadline_at: there is no window to describe (D-93)")
	}
	if hasKey(data, "remaining_ms") {
		t.Error("a Dutch tick carries remaining_ms: there is no window to count down (D-94)")
	}
	if got := engine.Rev(); got != rev {
		t.Errorf("revision = %d after a tick, want the unchanged %d", got, rev)
	}
}

// TestDutchFloorDoesNotSettle is D-99/DTCH-03's floor contract at the ticker:
// reaching the floor is NOT an outcome. The run stays LIVE, the price stays
// pinned at floor_price, and every beat keeps sending a tick — until the
// presenter's force-end, which settles unsold at price 0 and moves no budget
// (D-99/D-102/ECON-03).
func TestDutchFloorDoesNotSettle(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	// Twenty intervals in: long past the floor (the fixture floors after six).
	clock := &testClock{t: base.Add(20 * time.Second)}
	h, engine, _ := newLiveDutchHub(t, clock, base.UnixMilli(), 1_000_000)

	conn, snapshot := dutchAcceptDial(t, h)
	budgetBefore := meOf(t, snapshot).Budget

	for i := 0; i < 2; i++ {
		h.tick()
		beat := readType(t, conn, TypePriceTick)
		if got := objectInt(t, frameData(t, beat), "current_price"); got != dutchFloorPrice {
			t.Errorf("beat %d current_price = %d, want the pinned floor %d (D-91)", i, got, dutchFloorPrice)
		}
		if got := engine.Phase(); got != game.PhaseLive {
			t.Fatalf("phase after beat %d = %s, want LIVE: the floor is not an outcome (D-99)", i, got)
		}
	}
	if got := engine.Rev(); got != 1 {
		t.Errorf("revision = %d after two floor beats, want the seeded 1: a beat is not a mutation (D-58)", got)
	}

	// The presenter ends it: unsold, price 0, nobody charged (D-102/ECON-03).
	// The force-end travels from the admin socket; the settle's own frames are
	// read from the PLAYER's socket, whose `me` block is what proves no budget
	// moved (an admin snapshot carries no identity).
	admin := joinRole(t, h, RoleAdmin)
	writeFrame(t, admin, `{"type":"admin.item.force_end"}`)
	frames := expectFrames(t, conn, TypeItemSettled, TypeLeaderboard, TypeStateSnapshot)

	settled := dataObject(t, frames[0], "settle")
	if got := frameString(t, settled, "outcome"); got != "unsold" {
		t.Errorf("force-ended outcome = %q, want unsold: nobody accepted (D-99/D-102)", got)
	}
	if got := objectInt(t, settled, "price"); got != 0 {
		t.Errorf("force-ended price = %d, want 0 (ECON-03)", got)
	}
	if raw, ok := settled["winner"]; !ok || string(raw) != "null" {
		t.Errorf("force-ended winner = %s, want an explicit null", raw)
	}
	if got := meOf(t, frames[2]).Budget; got != budgetBefore {
		t.Errorf("a force-end moved a budget: %d -> %d, want the unchanged %d (ECON-03)", budgetBefore, got, budgetBefore)
	}
	if got := engine.Phase(); got != game.PhaseSettled {
		t.Errorf("phase after the force-end = %s, want SETTLED", got)
	}
}
