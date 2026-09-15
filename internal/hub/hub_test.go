package hub

import (
	"testing"
	"time"
)

// This file holds the hub-level contracts that are not about one specific
// transport case: D-40's policy numbers (declared in 02-01 and, until now,
// asserted nowhere), the keepalive mechanism that reclaims a peer which stopped
// answering pings, and the fan-out's basic reach.

// TestDefaultPolicyPinsTheD40Numbers closes the loop 02-01 left open: the
// liveness timings and the FIFO depth existed only as declared constants. They
// are written here as literals — not read back from the struct — so changing a
// production number has to be a deliberate edit that breaks this test, the same
// rule the wire contract follows (D-24).
func TestDefaultPolicyPinsTheD40Numbers(t *testing.T) {
	if got := DefaultPolicy.SendBuffer; got != 64 {
		t.Errorf("DefaultPolicy.SendBuffer = %d, want 64 (D-40: 20 clients × 64 small payloads is negligible)", got)
	}
	if got := DefaultPolicy.ReadLimit; got != 4<<10 {
		t.Errorf("DefaultPolicy.ReadLimit = %d, want %d (D-40: the Phase 1 frame cap, kept)", got, 4<<10)
	}
	if got := DefaultPolicy.PongWait; got != 60*time.Second {
		t.Errorf("DefaultPolicy.PongWait = %s, want 60s (D-40)", got)
	}
	if got := DefaultPolicy.PingPeriod; got != 25*time.Second {
		t.Errorf("DefaultPolicy.PingPeriod = %s, want 25s (D-40: well inside Cloudflare's ~100 s idle kill)", got)
	}
	if got := DefaultPolicy.WriteWait; got != 10*time.Second {
		t.Errorf("DefaultPolicy.WriteWait = %s, want 10s (D-40)", got)
	}
	if DefaultPolicy.PingPeriod >= DefaultPolicy.PongWait {
		t.Errorf("ping period %s is not well under the pong window %s: a healthy peer would be reclaimed between pings",
			DefaultPolicy.PingPeriod, DefaultPolicy.PongWait)
	}
}

// TestKeepaliveReclaimsASilentPeer pins D-40's mechanism, not its numbers: a
// peer that stops answering pings (phone suspended, Wi-Fi drop, NAT rebind) is
// reclaimed at the pong deadline instead of holding a goroutine and a file
// descriptor for the rest of the presentation.
//
// The peer here is a real socket that simply stops reading — gorilla only
// answers a ping when its read loop is running — so no pong ever arrives and the
// server's read deadline must fire. The test policy's 2 s window is what makes
// this observable without waiting out the production 60 s.
func TestKeepaliveReclaimsASilentPeer(t *testing.T) {
	h, _ := newTestHubWithPolicy(t, testPolicy())
	conn := dialWS(t, h)
	join(t, conn, `{"type":"join","role":"screen"}`)

	// Stop reading and outlive the pong window: the server's pings go
	// unanswered because this side never runs its read loop.
	time.Sleep(3 * time.Second)

	// The next read must fail: the server closed the connection when the pong
	// window expired.
	_ = conn.SetReadDeadline(time.Now().Add(testIOTimeout))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("the connection was still alive after the pong window expired; a silent peer is never reclaimed (D-40)")
	}
}

// TestFanOutReachesEveryJoinedClient pins the fan-out's reach and its one
// exclusion: a mutation is delivered to every connection that has joined,
// including the one that caused it, and never to a connection that has not
// completed a handshake (its role projection does not exist yet).
func TestFanOutReachesEveryJoinedClient(t *testing.T) {
	h, _ := newTestHub(t)

	screen := dialWS(t, h)
	_, before := join(t, screen, `{"type":"join","role":"screen"}`)

	silent := dialWS(t, h) // upgraded, never joined: it must receive nothing

	player := dialWS(t, h)
	join(t, player, `{"type":"join","role":"player","nickname":"An"}`)

	// The join is a mutation, so both joined connections see a fresh snapshot at
	// the new revision. (The joining player sees the handshake pair and then the
	// fan-out of the very mutation it caused — the same revision, one more seq.)
	after := readType(t, screen, TypeStateSnapshot)
	if after.Rev != before.Rev+1 {
		t.Errorf("screen rev after the mutation = %d, want %d (one mutation, one revision)", after.Rev, before.Rev+1)
	}
	if after.Seq <= before.Seq {
		t.Errorf("screen seq = %d after the mutation, want it past the handshake's %d", after.Seq, before.Seq)
	}
	if got := readType(t, player, TypeStateSnapshot); got.Rev != after.Rev {
		t.Errorf("the joining player's fan-out rev = %d, want %d (both clients see one mutation at one revision)", got.Rev, after.Rev)
	}

	// The unjoined connection has no role, so it is not part of any fan-out.
	if env, err := readFrameErr(silent); err == nil {
		t.Errorf("an unjoined connection received %q; the fan-out must skip connections without a role", env.Type)
	}
}
