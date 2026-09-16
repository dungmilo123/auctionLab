package hub

import (
	"fmt"
	"testing"
	"time"
)

// This file is the supersede contract (SESS-03 / D-31): a second connection
// with the same token replaces the first, the first observes close code 4001,
// and — the part that is easy to get wrong — the first connection's teardown
// can never evict its successor or flip the player's presence (research
// Pitfall 5).

// TestSupersedeClosesOldWith4001 pins the replacement itself: the same token on
// a new socket wins the identity, and the peer that lost it is told so with the
// dedicated close code rather than a silent drop.
func TestSupersedeClosesOldWith4001(t *testing.T) {
	h, engine := newTestHub(t)

	first := dialWS(t, h)
	hello, _ := join(t, first, `{"type":"join","role":"player","nickname":"An"}`)
	id := frameString(t, frameData(t, hello), "player")
	token := frameString(t, frameData(t, hello), "token")

	second := dialWS(t, h)
	secondHello, _ := join(t, second, fmt.Sprintf(`{"type":"join","role":"player","token":%q}`, token))
	if got := frameString(t, frameData(t, secondHello), "player"); got != id {
		t.Fatalf("the superseding join resolved to %q, want the same identity %q (D-30 (1): the token wins)", got, id)
	}

	if code := readCloseCode(t, first); code != 4001 {
		t.Errorf("the superseded peer observed close code %d, want 4001 (D-31)", code)
	}
	if got := engine.PlayerCount(); got != 1 {
		t.Errorf("player count = %d after a supersede, want 1 (a resume creates nothing)", got)
	}
}

// TestSupersedeTeardownDoesNotEvictSuccessor is research Pitfall 5's test: the
// dying connection must not delete the registration by player id, and must not
// mark the player disconnected on its way out. Both failures look identical
// from the client — "my page is live but I receive nothing" — and only a case
// that observes the registry AFTER the old teardown has run can catch them.
func TestSupersedeTeardownDoesNotEvictSuccessor(t *testing.T) {
	h, _ := newTestHub(t)

	first := dialWS(t, h)
	hello, _ := join(t, first, `{"type":"join","role":"player","nickname":"An"}`)
	id := frameString(t, frameData(t, hello), "player")
	token := frameString(t, frameData(t, hello), "token")

	// The successor's pointer is the one that must survive: compare-and-remove
	// is by identity, so a teardown that removed by player id would leave the
	// registry pointing at the wrong client (or at nothing).
	loser := h.registry.lookup(id)
	if loser == nil {
		t.Fatal("the first connection was never registered; the registry is not wired into the join path")
	}

	second := dialWS(t, h)
	secondHello, _ := join(t, second, fmt.Sprintf(`{"type":"join","role":"player","token":%q}`, token))
	if got := frameString(t, frameData(t, secondHello), "player"); got != id {
		t.Fatalf("the superseding join resolved to %q, want the same identity %q (D-30 (1): the token wins)", got, id)
	}
	if code := readCloseCode(t, first); code != 4001 {
		t.Fatalf("the superseded peer observed close code %d, want 4001 (D-31)", code)
	}

	// The superseded connection's own goroutine runs the teardown after its
	// socket has ended, so the only way to observe it is to wait for it: the
	// registry must converge on exactly one live holder of this identity, and
	// it must not be the pointer that was just closed.
	if !waitFor(2*time.Second, func() bool { return clientsFor(h, id) == 1 }) {
		t.Fatalf("the registry holds %d connections for %q after the supersede, want 1 — the old teardown evicted its successor or the successor was never registered",
			clientsFor(h, id), id)
	}
	winner := h.registry.lookup(id)
	if winner == nil {
		t.Fatal("the identity has no registered connection after the supersede")
	}
	if winner == loser {
		t.Error("the registry still points at the superseded connection; the successor's registration was evicted (D-31)")
	}

	// Presence must still read connected: the old teardown must not have run
	// Disconnect for a registration it no longer owned (D-31/D-37).
	observer := dialWS(t, h)
	_, observerSnapshot := join(t, observer, `{"type":"join","role":"screen"}`)
	if !lobbyConnected(t, observerSnapshot, "An") {
		t.Error("the lobby shows \"An\" as disconnected after the superseded teardown; compare-and-remove by pointer identity is missing (D-31)")
	}
}

// TestEndPlayerSessionEndsEveryHolder is WR-05's regression: D-78's
// clear_players and D-80's release end an identity through endPlayerSession,
// and resolving it by a single registry lookup is not enough — a supersede
// leaves the old connection REGISTERED until its own teardown runs, so during
// that window the registry holds two connections under one id and lookup
// returns the OLDEST, i.e. the socket that is already closed.
//
// The duplicate is forced through the white-box identity publish (the entry
// point the real handshake uses) rather than by racing the real window, which is
// a goroutine hop wide and cannot be held open from a test: both peers are live
// sockets that carry the id, and one call must end BOTH. With the pre-fix
// lookup, only the first — the stale one — observes 4002, and the live phone is
// left rendering an identity the presenter ended.
func TestEndPlayerSessionEndsEveryHolder(t *testing.T) {
	h, _ := newTestHub(t)

	first := dialWS(t, h)
	anHello, _ := join(t, first, `{"type":"join","role":"player","nickname":"An"}`)
	anID := frameString(t, frameData(t, anHello), "player")

	second := dialWS(t, h)
	binhHello, _ := join(t, second, `{"type":"join","role":"player","nickname":"Binh"}`)
	binhID := frameString(t, frameData(t, binhHello), "player")

	// The state a supersede leaves for the length of the old connection's
	// teardown hop: two registered connections claiming one identity. The
	// publish is an atomic store, so forcing it here is race-free.
	for _, c := range h.registry.snapshot() {
		if c.playerID() == binhID {
			c.setIdentity(RolePlayer, anID)
		}
	}
	if got := clientsFor(h, anID); got != 2 {
		t.Fatalf("the registry holds %d connections for %q, want the forced 2 — the duplicate holder was never published", got, anID)
	}

	h.endPlayerSession(anID)

	if code := readCloseCode(t, first); code != 4002 {
		t.Errorf("the older holder observed close code %d, want 4002 (D-78/D-80 deliberate cleanup)", code)
	}
	if code := readCloseCode(t, second); code != 4002 {
		t.Errorf("the newer holder observed close code %d, want 4002: endPlayerSession ended only the first registry match (WR-05)", code)
	}
}

// clientsFor counts the registered connections that carry one player id. It is
// the white-box half of the D-31 contract: the black-box half (the successor
// keeps receiving fan-out) needs the enqueue path and belongs to the ordering
// tests.
func clientsFor(h *Hub, playerID string) int {
	count := 0
	for _, c := range h.registry.snapshot() {
		if c.playerID() == playerID {
			count++
		}
	}
	return count
}
