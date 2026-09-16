package hub

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
	"auctionlab/internal/store"
)

// This file owns the Phase 6 command surface at the transport boundary:
// `sealed.submit {amount}` is a player-only command (D-111) whose success is a
// PRIVATE `sealed.received` carrying the count and nothing else (SEAL-01), and
// whose refusals travel as the ordinary private `bid.rejected` frame with the
// engine's own reason (D-112) — no new wire type is added by this phase (D-24).
//
// The cases are scripted exact frame sequences, like dutch_test.go: the ORDER of
// one mutation's siblings and the revision they share are part of the contract
// (D-21/D-57).

// The seeded sealed lot: a wall-clock window (D-107) whose deadline the runtime
// row persists, with anti-snipe disabled because a sealed window is never
// extended.
const (
	sealedItemID      = "it-sealed"
	sealedPlayerID    = "p-an"
	sealedPlayerToken = "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	sealedStartPrice  = int64(100_000)
	sealedDurationMS  = int64(30_000)
	sealedTruthValue  = int64(350_000)
)

// newLiveSealedHub builds a hub whose engine BOOTS onto a file that already
// carries a LIVE sealed run and one registered player (whose token
// `sealedPlayerToken` resumes it).
//
// The item can only be started through `admin.item.start` once the sealed
// per-type live guard lands in a later plan of this phase, and the wire
// contracts below are this plan's subject — so the fixture seeds the durable
// rows through the store (exactly what the engine package's mustOpenSealedRun
// does) and lets the real boot validate and resume them (D-101).
func newLiveSealedHub(t *testing.T, clock *testClock, startedAt, budget int64) (*Hub, *game.Engine, *store.Store) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "auction.db"))
	if err != nil {
		t.Fatalf("opening the seeded database: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := store.Schema(st.DB); err != nil {
		t.Fatalf("applying the schema: %v", err)
	}
	if err := store.Bootstrap(st.DB, clock.now()); err != nil {
		t.Fatalf("bootstrapping the seeded game row: %v", err)
	}

	it := store.ItemRow{
		ID: sealedItemID, Name: "Tranh", AuctionType: "sealed_first", TruthValue: sealedTruthValue,
		StartPrice: sealedStartPrice, DurationMS: int64Ptr(sealedDurationMS),
		AntiSnipeMode: "disabled", Status: "live",
	}
	an := store.PlayerRow{
		ID: sealedPlayerID, Nickname: "An", NicknameKey: "an", Token: sealedPlayerToken,
		Budget: budget, JoinedAt: startedAt, LastSeenAt: startedAt,
	}
	if err := st.Commit(store.Mutation{
		Phase: "LIVE", CurrentItemID: it.ID, Revision: 1, UpdatedAt: startedAt,
		Items:   []store.ItemRow{it},
		Players: []store.PlayerRow{an},
		Runtime: &store.RuntimeRow{
			ItemID: it.ID, StartedAt: startedAt,
			DeadlineAt: int64Ptr(startedAt + sealedDurationMS), Revision: 1,
		},
	}); err != nil {
		t.Fatalf("seeding the live sealed run: %v", err)
	}

	engine, err := game.New(game.Config{
		Store: st,
		Now:   clock.now,
		Fatal: func(err error) { t.Errorf("engine reported an unrecoverable commit failure: %v", err) },
	})
	if err != nil {
		t.Fatalf("booting on the seeded sealed file: %v", err)
	}

	h := newHub(engine, testPolicyWithBuffer(64), testAdminKey)
	h.now = clock.now
	engine.SetPublisher(h.Fanout)
	return h, engine, st
}

// sealedSubmitDial dials the seeded player and completes the D-30 token resume.
func sealedSubmitDial(t *testing.T, h *Hub) (*websocket.Conn, frame) {
	t.Helper()

	conn := dialWS(t, h)
	_, snapshot := join(t, conn, `{"type":"join","role":"player","token":"`+sealedPlayerToken+`"}`)
	expectFrames(t, conn, TypeStateSnapshot)
	return conn, snapshot
}

// TestSealedSubmitOnTheWire is SEAL-01's happy path at the transport edge: ONE
// real player frame is one mutation — revision +1, one effective row and one
// `sealed_edit` audit row in the same transaction — answered with the private
// `sealed.received` count (no amount) and then the ordinary snapshot.
func TestSealedSubmitOnTheWire(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	clock := &testClock{t: base.Add(1 * time.Second)} // one second into the window
	h, engine, st := newLiveSealedHub(t, clock, base.UnixMilli(), 1_000_000)

	conn, _ := sealedSubmitDial(t, h)
	revBefore := engine.Rev()

	writeFrame(t, conn, `{"type":"sealed.submit","amount":150000}`)
	frames := expectFrames(t, conn, TypeSealedReceived, TypeStateSnapshot)

	// SEAL-01: the confirmation carries the count and NOTHING else — the key set
	// is the contract, because a price that never travels cannot leak.
	received := frameData(t, frames[0])
	if len(received) != 2 {
		t.Errorf("sealed.received keys = %v, want exactly submitted+total (SEAL-01: the confirmation contains no price)", received)
	}
	if got := objectInt(t, received, "submitted"); got != 1 {
		t.Errorf("sealed.received.submitted = %d, want 1", got)
	}
	if got := objectInt(t, received, "total"); got != 1 {
		t.Errorf("sealed.received.total = %d, want 1", got)
	}
	if hasKey(received, "amount") {
		t.Error("sealed.received carries an amount: the confirmation must never reveal the bid (SEAL-01)")
	}

	// One mutation, one revision: both siblings carry it, and the engine agrees.
	rev := frames[0].Rev
	for i, env := range frames {
		if env.Rev != rev {
			t.Errorf("frame %d (%s) carries rev %d, want the mutation's %d (D-21)", i, env.Type, env.Rev, rev)
		}
	}
	if rev != revBefore+1 {
		t.Errorf("the submit committed revision %d, want %d — one submit is one mutation (D-110)", rev, revBefore+1)
	}
	if got := engine.Rev(); got != rev {
		t.Errorf("engine revision = %d, want %d", got, rev)
	}

	// Durable evidence: the effective row and its audit row.
	sheets, err := st.LoadSealedBids()
	if err != nil {
		t.Fatalf("loading the sealed bids: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("the wire submit wrote %d effective row(s), want exactly 1: %+v", len(sheets), sheets)
	}
	if sheets[0].Amount != 150_000 {
		t.Errorf("effective amount = %d, want 150000", sheets[0].Amount)
	}

	trail, err := st.LoadBids()
	if err != nil {
		t.Fatalf("loading the bid audit: %v", err)
	}
	if len(trail) != 1 {
		t.Fatalf("the wire submit appended %d audit row(s), want exactly 1: %+v", len(trail), trail)
	}
	if trail[0].Kind != "sealed_edit" || trail[0].Amount != 150_000 {
		t.Errorf("audit row = %+v, want kind sealed_edit at 150000 (D-109)", trail[0])
	}
}

// TestSealedSubmitRoleGate pins D-111's role gate at the transport edge: the
// command is player-only, so an admin or screen sender is answered with the
// frozen `bad_message` and never reaches the engine (H12).
func TestSealedSubmitRoleGate(t *testing.T) {
	t.Run("an admin command is bad_message", func(t *testing.T) {
		h, _ := newTestHub(t)
		admin := joinRole(t, h, RoleAdmin)

		writeFrame(t, admin, `{"type":"sealed.submit","amount":150000}`)
		assertErrorCode(t, readFrame(t, admin), CodeBadMessage)
	})

	t.Run("a screen command is bad_message", func(t *testing.T) {
		h, _ := newTestHub(t)
		screen := joinRole(t, h, RoleScreen)

		writeFrame(t, screen, `{"type":"sealed.submit","amount":150000}`)
		assertErrorCode(t, readFrame(t, screen), CodeBadMessage)
	})
}
