package hub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
)

// This file owns the D-56 ticker contract at the transport boundary: a beat is a
// `price.tick` carrying the server's own timing numbers at the CURRENT revision
// (never a bump), the mutation path emits one immediately when the numbers move
// (entering LIVE, an accepted bid), and a beat whose deadline has passed settles
// through the engine's mutation path instead of emitting a stale LIVE tick.
//
// The ticker goroutine is never what the assertions wait on: `h.tick()` is one
// deterministic beat, exactly as PATTERNS/H16 prescribes, so no case sleeps on a
// production cadence.

// startLiveItem creates one English item through the engine's real mutation path
// and drives it to LIVE (D-65: upsert → start → start), returning its id. Using
// the engine API rather than a socket keeps Task 1 transport-level: the command
// routing a socket exercises lands in the next task.
func startLiveItem(t *testing.T, e *game.Engine, durationMS int64) string {
	t.Helper()

	increment := int64(10_000)
	duration := durationMS
	if err := e.ItemUpsert(game.ItemInput{
		Name:          "Phase 3 lot",
		AuctionType:   "english",
		TruthValue:    350_000,
		StartPrice:    100_000,
		MinIncrement:  &increment,
		DurationMS:    &duration,
		AntiSnipeMode: "disabled",
	}); err != nil {
		t.Fatalf("creating the test item: %v", err)
	}

	view, ok := e.Snapshot(game.RoleAdmin, "").(game.AdminView)
	if !ok || len(view.Items) != 1 {
		t.Fatalf("admin snapshot items = %+v, want exactly the item just created", view.Items)
	}
	id := view.Items[0].ID

	if err := e.StartItem(id); err != nil {
		t.Fatalf("start (LOBBY → ITEM_INTRO): %v", err)
	}
	if err := e.StartItem(id); err != nil {
		t.Fatalf("start (ITEM_INTRO → LIVE): %v", err)
	}
	return id
}

// expectFrames reads exactly the named frame types, in order. The ORDER is the
// contract: one mutation's siblings share a revision and a client must see them
// in the order the mutation recorded them (D-57).
func expectFrames(t *testing.T, conn *websocket.Conn, types ...string) []frame {
	t.Helper()

	frames := make([]frame, 0, len(types))
	for i, want := range types {
		env := readFrame(t, conn)
		if env.Type != want {
			t.Fatalf("frame %d = %q, want %q (the fan-out order is the contract)", i, env.Type, want)
		}
		frames = append(frames, env)
	}
	return frames
}

// intField reads one integer field out of a decoded frame's data object.
func intField(t *testing.T, env frame, key string) int64 {
	t.Helper()

	var value int64
	if err := json.Unmarshal(dataField(t, env, key), &value); err != nil {
		t.Fatalf("data.%s is not an integer: %v", key, err)
	}
	return value
}

// TestTickCarriesTimingWithoutBumpingRevision pins D-56/D-58 on a real socket:
// the immediate tick after entering LIVE and the periodic beat both carry the
// engine's own numbers at the CURRENT revision, and a beat past the deadline
// settles exactly once instead of emitting a stale LIVE frame.
func TestTickCarriesTimingWithoutBumpingRevision(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	clock := &testClock{t: base}
	h, engine := newTestHubWithClock(t, testPolicyWithBuffer(256), clock)

	conn := dialWS(t, h)
	join(t, conn, `{"type":"join","role":"player","nickname":"An"}`)
	// A join is a mutation: the handshake pair is followed by the fan-out of
	// that same mutation (the joining player is already registered), and with no
	// typed event of its own the snapshot IS the frame (H2).
	expectFrames(t, conn, TypeStateSnapshot)

	// The whole admin flow through the engine, one mutation per frame group:
	// upsert (D-66's snapshot), start → ITEM_INTRO, start → LIVE + the D-56
	// immediate tick. The order IS the contract (D-57).
	_ = startLiveItem(t, engine, 30_000)
	frames := expectFrames(t, conn,
		TypeStateSnapshot, // item.upsert
		TypePhaseChanged, TypeItemIntro, TypeStateSnapshot,
		TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)

	immediate := frames[7]
	rev := engine.Rev()
	if immediate.Rev != rev {
		t.Fatalf("immediate tick rev = %d, want the current %d (a tick never bumps it, D-58)", immediate.Rev, rev)
	}
	if got, want := intField(t, immediate, "remaining_ms"), int64(30_000); got != want {
		t.Errorf("immediate tick remaining_ms = %d, want %d (deadline is base + duration)", got, want)
	}
	if got, want := intField(t, immediate, "deadline_at"), base.Add(30*time.Second).UnixMilli(); got != want {
		t.Errorf("immediate tick deadline_at = %d, want %d", got, want)
	}

	// A direct beat after six seconds of injected time carries the new remaining
	// window, the same revision, and moves nothing else.
	clock.t = base.Add(5 * time.Second)
	h.tick()
	beat := readType(t, conn, TypePriceTick)
	if beat.Seq <= immediate.Seq {
		t.Errorf("beat seq = %d, want it past the immediate tick's %d", beat.Seq, immediate.Seq)
	}
	if beat.Rev != rev {
		t.Errorf("beat rev = %d, want the unchanged %d (D-58)", beat.Rev, rev)
	}
	if got, want := intField(t, beat, "remaining_ms"), int64(25_000); got != want {
		t.Errorf("beat remaining_ms = %d, want %d (server-computed from the injected clock)", got, want)
	}
	if got := engine.Rev(); got != rev {
		t.Errorf("engine revision after a tick = %d, want %d (a tick is not a mutation)", got, rev)
	}

	// Advancing past the deadline makes the NEXT beat settle: one mutation, one
	// revision, and the settle's own three frames — never a LIVE tick.
	clock.t = base.Add(31 * time.Second)
	h.tick()

	if got := engine.Phase(); got != game.PhaseSettled {
		t.Fatalf("phase after a due beat = %s, want SETTLED (D-55a)", got)
	}
	if got, want := engine.Rev(), rev+1; got != want {
		t.Fatalf("revision after the settle = %d, want exactly one bump to %d (Pitfall 2: a double settle)", got, want)
	}
	expectFrames(t, conn, TypeItemSettled, TypeLeaderboard, TypeStateSnapshot)

	// A second beat is a clean no-op, and the next mutation is the next frame on
	// the socket: a settled beat must not leave a stale LIVE tick queued.
	h.tick()
	late := dialWS(t, h)
	writeFrame(t, late, `{"type":"join","role":"player","nickname":"Binh"}`)
	if next := readFrame(t, conn); next.Type != TypeStateSnapshot {
		t.Fatalf("the frame after a settled beat = %q, want the next mutation's %q — a due beat must not emit a LIVE tick",
			next.Type, TypeStateSnapshot)
	}
}

// TestTickCadenceAndImmediateTicks pins the two producers of a tick: the
// mutation path emits one immediately (entering LIVE, an accepted bid), and the
// single StartTicker goroutine beats repeatedly at the current revision.
func TestTickCadenceAndImmediateTicks(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	clock := &testClock{t: base}
	policy := testPolicyWithBuffer(256)
	policy.TickPeriod = 10 * time.Millisecond
	h, engine := newTestHubWithClock(t, policy, clock)

	player := dialWS(t, h)
	hello, _ := join(t, player, `{"type":"join","role":"player","nickname":"An"}`)
	playerID := frameString(t, frameData(t, hello), "player")
	expectFrames(t, player, TypeStateSnapshot) // the player's own join fan-out

	screen := dialWS(t, h)
	join(t, screen, `{"type":"join","role":"screen"}`)
	// A screen join is not a mutation (no identity, no presence), so it fans
	// out nothing: the next frames on either socket are the item mutations.

	// No periodic goroutine yet: every tick below is the mutation path's own.
	startLiveItem(t, engine, 60_000)
	for _, conn := range []*websocket.Conn{player, screen} {
		expectFrames(t, conn,
			TypeStateSnapshot, // item.upsert
			TypePhaseChanged, TypeItemIntro, TypeStateSnapshot,
			TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)
	}

	// An accepted bid is a mutation: bid.accepted, then the immediate tick
	// carrying the price the bid just set and the new minimum.
	rev := engine.Rev()
	if err := engine.PlaceBid(playerID, 100_000, nil); err != nil {
		t.Fatalf("the first bid at the start price: %v", err)
	}
	expectFrames(t, player, TypeBidAccepted, TypePriceTick)
	bidFrames := expectFrames(t, screen, TypeBidAccepted, TypePriceTick)
	for i, env := range bidFrames {
		if env.Rev != rev+1 {
			t.Errorf("bid sibling %d (%s) rev = %d, want the mutation's %d", i, env.Type, env.Rev, rev+1)
		}
	}
	if got := intField(t, bidFrames[1], "current_price"); got != 100_000 {
		t.Errorf("immediate tick current_price = %d, want the accepted bid 100000", got)
	}
	if got := intField(t, bidFrames[1], "min_next_bid"); got != 110_000 {
		t.Errorf("immediate tick min_next_bid = %d, want 110000 (the D-59 increment)", got)
	}

	// The cadence: one goroutine, repeated beats, always the current revision.
	stop := h.StartTicker()
	t.Cleanup(stop)

	previous := bidFrames[1].Seq
	for i := 0; i < 3; i++ {
		env := readType(t, player, TypePriceTick)
		if env.Rev != engine.Rev() {
			t.Errorf("periodic tick %d rev = %d, want the current %d (a tick never bumps it, D-58)", i, env.Rev, engine.Rev())
		}
		if env.Seq <= previous {
			t.Errorf("periodic tick %d seq = %d, want it past %d (one FIFO, one sequence)", i, env.Seq, previous)
		}
		previous = env.Seq
	}
	stop()
}

// TestTickPeriodIsPinned closes the loop on the D-56 number the same way
// TestDefaultPolicyPinsTheD40Numbers does: as a literal, so changing the
// production cadence has to be a deliberate edit.
func TestTickPeriodIsPinned(t *testing.T) {
	if got := DefaultPolicy.TickPeriod; got != 250*time.Millisecond {
		t.Errorf("DefaultPolicy.TickPeriod = %s, want 250ms (D-56: the client interpolates between beats)", got)
	}
}
