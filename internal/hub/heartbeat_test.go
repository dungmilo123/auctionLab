package hub

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
)

// This file owns D-132's heartbeat contract at the transport boundary: one beat
// reaches every joined role in every game phase, carries exactly the server's
// injected millisecond clock, takes a real seq on each connection's own FIFO,
// moves no revision, and never inflates the drop/resync accounting.
//
// The goroutine is never what the assertions wait on: heartbeat() is one
// deterministic beat — the ticker.go rule — so no case sleeps on the 25-second
// production cadence.

// assertHeartbeat pins the beat's whole contract for one recipient: the frozen
// frame type, the control-plane revision rule, the exact payload shape, and the
// wall-clock value the injected hub clock produced.
func assertHeartbeat(t *testing.T, env frame, wantTS int64, wantRev int64) {
	t.Helper()

	if env.Type != TypeHeartbeat {
		t.Fatalf("frame type = %q, want %q", env.Type, TypeHeartbeat)
	}
	if env.Rev != wantRev {
		t.Errorf("heartbeat rev = %d, want the unchanged %d (a beat is not a mutation, D-35)", env.Rev, wantRev)
	}
	data := frameData(t, env)
	if len(data) != 1 {
		t.Errorf("heartbeat data keys = %v, want exactly [ts] (D-132: nothing role-specific, nothing stateful)", data)
	}
	if got := intField(t, env, "ts"); got != wantTS {
		t.Errorf("heartbeat ts = %d, want %d (the server's injected clock)", got, wantTS)
	}
}

// TestHeartbeat is D-132's whole mechanism.
func TestHeartbeat(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)

	t.Run("one beat reaches every joined role with the injected clock's ts", func(t *testing.T) {
		clock := &testClock{t: base}
		h, engine := newTestHubWithClock(t, testPolicyWithBuffer(64), clock)

		admin := dialWS(t, h)
		join(t, admin, adminJoinJSON)
		screen := dialWS(t, h)
		join(t, screen, `{"type":"join","role":"screen"}`)
		player := dialWS(t, h)
		join(t, player, `{"type":"join","role":"player","nickname":"An"}`)

		// The player's join fans one snapshot to every joined connection (the
		// D-33 edge), so each socket must consume it before the beat.
		joined := []struct {
			name string
			conn *websocket.Conn
		}{
			{"admin", admin},
			{"screen", screen},
			{"player", player},
		}
		for _, c := range joined {
			readType(t, c.conn, TypeStateSnapshot)
		}

		rev := engine.Rev()
		h.heartbeat()

		for _, c := range joined {
			env := readType(t, c.conn, TypeHeartbeat)
			if env.Seq != 4 {
				t.Errorf("%s: heartbeat seq = %d, want 4 (the beat takes a real seq on this connection's own FIFO)", c.name, env.Seq)
			}
			assertHeartbeat(t, env, base.UnixMilli(), rev)
		}

		if got := engine.Rev(); got != rev {
			t.Errorf("engine revision after a beat = %d, want the unchanged %d (D-132: a beat never moves game state)", got, rev)
		}
		if got := h.Drops(); got != 0 {
			t.Errorf("hub drops after a beat = %d, want 0 (a beat must not inflate the D-41 accounting)", got)
		}
		if got := h.Resyncs(); got != 0 {
			t.Errorf("hub resyncs after a beat = %d, want 0", got)
		}
	})

	t.Run("the beat runs in every phase, always at the current revision", func(t *testing.T) {
		for _, phase := range game.AllPhases() {
			t.Run(string(phase), func(t *testing.T) {
				clock := &testClock{t: base}
				h, engine := newTestHubWithClock(t, testPolicyWithBuffer(64), clock)

				conn := dialWS(t, h)
				join(t, conn, `{"type":"join","role":"screen"}`)
				drivePhase(t, engine, phase)

				if got := engine.Phase(); got != phase {
					t.Fatalf("phase = %s, want %s (the fixture failed to drive the engine)", got, phase)
				}
				rev := engine.Rev()

				h.heartbeat()
				env := readType(t, conn, TypeHeartbeat)
				assertHeartbeat(t, env, base.UnixMilli(), rev)

				if got := engine.Rev(); got != rev {
					t.Errorf("engine revision after a beat in %s = %d, want %d", phase, got, rev)
				}
			})
		}
	})

	t.Run("a beat storm does not inflate the drop and resync accounting", func(t *testing.T) {
		clock := &testClock{t: base}
		h, _ := newTestHubWithClock(t, testPolicyWithBuffer(8), clock)

		// A connection built directly: the assertion is about the beats landing
		// in the FIFO and being drained, exactly the TestStartTickerStopsTheBeat
		// shape (no socket, no reader deadline).
		c := &client{hub: h, send: make(chan Envelope, 8), done: make(chan struct{})}
		c.setIdentity(RoleScreen, "")
		h.registry.add(c)

		for i := 0; i < 10; i++ {
			h.heartbeat()
			types := drainSend(c)
			if len(types) != 1 || types[0] != TypeHeartbeat {
				t.Fatalf("beat %d enqueued %v, want exactly one %q", i, types, TypeHeartbeat)
			}
		}
		if got := h.Drops(); got != 0 {
			t.Errorf("hub drops after 10 drained beats = %d, want 0 (a beat must not arm the D-41 fence)", got)
		}
		if got := h.Resyncs(); got != 0 {
			t.Errorf("hub resyncs after 10 drained beats = %d, want 0", got)
		}
	})

	t.Run("StartHeartbeat beats on its own cadence and stops for good", func(t *testing.T) {
		policy := testPolicyWithBuffer(64)
		policy.HeartbeatPeriod = 5 * time.Millisecond
		h, _ := newTestHubWithClock(t, policy, &testClock{t: base})

		c := &client{hub: h, send: make(chan Envelope, 64), done: make(chan struct{})}
		c.setIdentity(RoleScreen, "")
		h.registry.add(c)

		stop := h.StartHeartbeat()
		if !waitFor(2*time.Second, func() bool { return len(c.send) >= 3 }) {
			t.Fatalf("the heartbeat produced %d beat(s) in 2s; it never started", len(c.send))
		}
		stop()

		// At most one beat can be in flight when stop() returns (the goroutine
		// finishes the beat it is inside, then sees the closed done channel). Let
		// it land, drain, and require silence across several periods.
		time.Sleep(20 * time.Millisecond)
		for len(c.send) > 0 {
			<-c.send
		}
		time.Sleep(40 * time.Millisecond) // eight periods at 5ms

		if got := len(c.send); got != 0 {
			t.Errorf("the heartbeat enqueued %d beat(s) after stop: the goroutine outlived its stop function (Pitfall 11)", got)
		}
		stop() // idempotent
	})
}

// drivePhase advances one engine to the wanted phase through the real mutation
// path, using the shipped admin commands (D-65): upsert → start → start → force
// end → next.
func drivePhase(t *testing.T, e *game.Engine, want game.Phase) {
	t.Helper()

	if want == game.PhaseLobby {
		return
	}

	increment := int64(10_000)
	duration := int64(30_000)
	if err := e.ItemUpsert(game.ItemInput{
		Name:          "Phase 7 lot",
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

	step := func(err error) {
		if err != nil {
			t.Fatalf("driving to %s: %v", want, err)
		}
	}
	step(e.StartItem(id)) // LOBBY → ITEM_INTRO
	if want == game.PhaseItemIntro {
		return
	}
	step(e.StartItem(id)) // ITEM_INTRO → LIVE
	if want == game.PhaseLive {
		return
	}
	step(e.ForceEnd()) // LIVE → SETTLED
	if want == game.PhaseSettled {
		return
	}
	step(e.NextItem()) // SETTLED → FINALE (no pending item left)
}
