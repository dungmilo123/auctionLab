package hub

import (
	"encoding/json"
	"testing"

	"github.com/gorilla/websocket"
)

// This file owns the tunnel.status control-plane contract at the transport
// boundary (D-123): the frame reaches the two roles that act on it and never a
// player (T-07-05), it takes a real seq on each recipient's own FIFO without
// moving the revision, and the runtime URL change that accompanies it fans each
// connection's own role projection at the CURRENT revision so the QR rotates
// immediately (D-128, research Pitfall 3).

// TestTunnelStatusBroadcast pins D-123's audience and sequencing rules on real
// sockets, and the same-revision snapshot fan-out D-128 needs, in one case: the
// two producers are wired together by cmd/server, so the frames a room sees must
// be checked together.
func TestTunnelStatusBroadcast(t *testing.T) {
	const tunnelURL = "https://tunnel-status-test.trycloudflare.com"

	h, engine := newTestHub(t)

	// One connection per role. Joins are ordered so the exact seq arithmetic
	// below is derivable: the player's join is a mutation and therefore fans one
	// snapshot to every joined connection (the D-33 edge), which each socket must
	// consume before the control frame arrives.
	admin := dialWS(t, h)
	join(t, admin, adminJoinJSON)

	screen := dialWS(t, h)
	join(t, screen, `{"type":"join","role":"screen"}`)

	player := dialWS(t, h)
	join(t, player, `{"type":"join","role":"player","nickname":"An"}`)

	// The player's join fan-out: one snapshot per joined connection, including
	// the joiner's own (its handshake snapshot was seq 2).
	joined := []struct {
		name string
		conn *websocket.Conn
	}{
		{"admin", admin},
		{"screen", screen},
		{"player", player},
	}
	for _, c := range joined {
		if env := readType(t, c.conn, TypeStateSnapshot); env.Seq != 3 {
			t.Fatalf("%s: the join fan-out arrived at seq %d, want 3 (one FIFO, one sequence)", c.name, env.Seq)
		}
	}

	rev := engine.Rev()

	// The control frame: admin and screen only, and each takes the next seq on
	// its own stream.
	h.BroadcastTunnelStatus(tunnelURL, true)

	for _, c := range joined[:2] {
		env := readType(t, c.conn, TypeTunnelStatus)
		if env.Seq != 4 {
			t.Errorf("%s: tunnel.status seq = %d, want 4 — the control frame is enqueued on the same lock-held FIFO as every other message", c.name, env.Seq)
		}
		if env.Rev != rev {
			t.Errorf("%s: tunnel.status rev = %d, want the unchanged %d (a transport fact is not a game mutation, D-35)", c.name, env.Rev, rev)
		}
		data := frameData(t, env)
		if got := frameString(t, data, "url"); got != tunnelURL {
			t.Errorf("%s: tunnel.status url = %q, want %q", c.name, got, tunnelURL)
		}
		var healthy bool
		if err := json.Unmarshal(data["healthy"], &healthy); err != nil {
			t.Fatalf("%s: tunnel.status healthy is not a bool: %v (%s)", c.name, err, env.Data)
		}
		if !healthy {
			t.Errorf("%s: tunnel.status healthy = false, want true", c.name)
		}
	}

	// The runtime URL change the same status produces in the engine fans each
	// connection's own role projection at the CURRENT revision — no bump, no
	// persistence (D-128). The player's NEXT frame is that snapshot, which is
	// also the proof it was never sent the control frame: had tunnel.status been
	// queued for it, that frame would be the one read here — and it would have
	// consumed seq 4.
	engine.SetTunnelStatus(tunnelURL, true)

	playerNext := readFrame(t, player)
	if playerNext.Type != TypeStateSnapshot {
		t.Fatalf("the player's next frame = %q, want %q — tunnel.status must never reach a player (T-07-05)", playerNext.Type, TypeStateSnapshot)
	}
	if playerNext.Seq != 4 {
		t.Errorf("the player's stream jumped to seq %d, want 4: a frame the player never receives must not consume its sequence", playerNext.Seq)
	}
	if playerNext.Rev != rev {
		t.Errorf("the player's runtime snapshot rev = %d, want the unchanged %d (D-128: the QR moves, the revision does not)", playerNext.Rev, rev)
	}

	// The projections the room renders from now carry the tunnel URL: the
	// projector's join_url (the QR text) and the admin's selection plus the rung
	// that won it.
	adminSnap := readType(t, admin, TypeStateSnapshot)
	screenSnap := readType(t, screen, TypeStateSnapshot)
	for _, c := range []struct {
		name string
		env  frame
	}{{"admin", adminSnap}, {"screen", screenSnap}} {
		if c.env.Seq != 5 {
			t.Errorf("%s: the runtime snapshot arrived at seq %d, want 5 (the control frame was 4)", c.name, c.env.Seq)
		}
		if c.env.Rev != rev {
			t.Errorf("%s: the runtime snapshot rev = %d, want the unchanged %d (D-128)", c.name, c.env.Rev, rev)
		}
	}
	if got := frameString(t, frameData(t, screenSnap), "join_url"); got != tunnelURL {
		t.Errorf("the projector's join_url = %q, want the tunnel URL %q — the QR is server-authoritative (D-84/D-128)", got, tunnelURL)
	}
	adminData := frameData(t, adminSnap)
	if got := frameString(t, adminData, "selected_join_url"); got != tunnelURL {
		t.Errorf("the admin's selected_join_url = %q, want the tunnel URL %q", got, tunnelURL)
	}
	if got := frameString(t, adminData, "join_url_source"); got != "tunnel" {
		t.Errorf("the admin's join_url_source = %q, want %q — the panel cannot derive which rung won (D-127)", got, "tunnel")
	}

	if got := engine.Rev(); got != rev {
		t.Errorf("engine revision after the status fan-out = %d, want %d (nothing was persisted, nothing was mutated)", got, rev)
	}
}
