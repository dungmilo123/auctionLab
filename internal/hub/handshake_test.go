package hub

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// This file is the transport contract of the real hub (D-23 / D-30 / D-31 /
// D-43): the three role handshakes, the nickname policy at the socket, the
// rejection cap, the token ladder, the malformed-frame paths and the
// cross-origin guard — every one of them asserted on a real WebSocket, never on
// a mock.
//
// The assertions are written against JSON literals rather than the production
// catalog on purpose (D-24 freezes the wire, not the Go identifiers): renaming
// a constant must not be able to move this contract silently.

// TestHandshakePerRole is the handshake's reason for existing (RT-07 / D-23):
// each of the three roles reaches hello at seq 1 and then a role snapshot at
// seq 2, both carrying the same revision because no mutation happened in
// between.
func TestHandshakePerRole(t *testing.T) {
	for _, role := range []string{"player", "screen", "admin"} {
		t.Run(role, func(t *testing.T) {
			h, _ := newTestHub(t)
			conn := dialWS(t, h)

			joinRequest := fmt.Sprintf(`{"type":"join","role":%q`, role)
			if role == "player" {
				joinRequest += `,"nickname":"An"`
			}
			joinRequest += "}"

			hello, snapshot := join(t, conn, joinRequest)
			if hello.Seq != 1 {
				t.Errorf("hello seq = %d, want 1 (D-21: seq starts at 1 on a fresh connection)", hello.Seq)
			}
			// Creating a player is a game mutation, so the player handshake is
			// the first revision; screen and admin mutate nothing (D-35).
			wantRev := int64(0)
			if role == "player" {
				wantRev = 1
			}
			if hello.Rev != wantRev {
				t.Errorf("hello rev = %d, want %d", hello.Rev, wantRev)
			}
			assertEnvelopeKeys(t, hello)
			if got := frameString(t, frameData(t, hello), "role"); got != role {
				t.Errorf("hello role = %q, want %q", got, role)
			}

			if snapshot.Seq != 2 {
				t.Errorf("snapshot seq = %d, want 2 (D-21: the snapshot is the second frame of the stream)", snapshot.Seq)
			}
			if snapshot.Rev != hello.Rev {
				t.Errorf("snapshot rev = %d, hello rev = %d, want one revision (D-23: no event can slip between the pair)",
					snapshot.Rev, hello.Rev)
			}
			assertEnvelopeKeys(t, snapshot)
			if got := frameString(t, frameData(t, snapshot), "phase"); got != "LOBBY" {
				t.Errorf("snapshot phase = %q, want \"LOBBY\" (PLAN.md §6: Phase 2 wires the machine only as far as LOBBY)", got)
			}
			// The role is observable in the projection's shape: a player view
			// carries `me`, the screen's is public, and the admin's is
			// secret-capable (D-25/D-26).
			data := frameData(t, snapshot)
			switch role {
			case "player":
				if _, ok := data["me"]; !ok {
					t.Error("player snapshot has no `me` block; the view must be self-sufficient (D-25)")
				}
			case "screen":
				if _, ok := data["me"]; ok {
					t.Error("screen snapshot carries a player identity (D-25)")
				}
			case "admin":
				if _, ok := data["sealed_bids"]; !ok {
					t.Error("admin snapshot has no sealed_bids block; AdminView is secret-capable by design (D-24/D-26)")
				}
			}
		})
	}
}

// TestPlayerHelloCarriesIdentity pins D-29's exception: hello is the only
// message that ever carries a token, and only the token of the player it is
// addressed to. Screen and admin handshakes carry no identity at all.
func TestPlayerHelloCarriesIdentity(t *testing.T) {
	h, _ := newTestHub(t)

	player := dialWS(t, h)
	hello, _ := join(t, player, `{"type":"join","role":"player","nickname":"An"}`)
	data := frameData(t, hello)

	id := frameString(t, data, "player")
	if id == "" {
		t.Error("player hello has no player id (D-23: a player handshake carries the identity)")
	}
	token := frameString(t, data, "token")
	if len(token) != 64 {
		t.Errorf("player hello token = %q (%d chars), want 64 hex characters (D-29)", token, len(token))
	}
	if got := frameString(t, data, "nickname"); got != "An" {
		t.Errorf("player hello nickname = %q, want %q", got, "An")
	}

	for _, role := range []string{"screen", "admin"} {
		conn := dialWS(t, h)
		env, _ := join(t, conn, fmt.Sprintf(`{"type":"join","role":%q}`, role))
		if _, ok := frameData(t, env)["token"]; ok {
			t.Errorf("%s hello carries a token; only a player handshake may (D-29)", role)
		}
		if _, ok := frameData(t, env)["player"]; ok {
			t.Errorf("%s hello carries a player id; screen and admin have no identity (D-23)", role)
		}
	}
}

// TestJoinRejectsDuplicateNickname pins SESS-01/D-28 at the socket: uniqueness
// is decided on the normalized, case-folded key, and a rejection is answered
// with the typed error instead of a broken connection.
func TestJoinRejectsDuplicateNickname(t *testing.T) {
	h, engine := newTestHub(t)

	holder := dialWS(t, h)
	first, _ := join(t, holder, `{"type":"join","role":"player","nickname":"An"}`)
	firstID := frameString(t, frameData(t, first), "player")

	dup := dialWS(t, h)
	// Whitespace padding and case folding are the same identity (D-27/D-28).
	writeFrame(t, dup, `{"type":"join","role":"player","nickname":"  an "}`)
	assertErrorCode(t, readFrame(t, dup), CodeNicknameTaken)

	if got := engine.PlayerCount(); got != 1 {
		t.Fatalf("player count = %d after a rejected join, want 1 (the collision must not create a row)", got)
	}

	// The rejected socket is still usable for a corrected nickname (D-23).
	second, snapshot := join(t, dup, `{"type":"join","role":"player","nickname":"Binh"}`)
	secondID := frameString(t, frameData(t, second), "player")
	if secondID == firstID {
		t.Error("the corrected join reused the first player's id")
	}
	if got := meOf(t, snapshot); got.ID != secondID {
		t.Errorf("snapshot me.id = %q, want the hello's player %q", got.ID, secondID)
	}
	if got := engine.PlayerCount(); got != 2 {
		t.Errorf("player count = %d after a successful retry, want 2", got)
	}
}

// TestJoinFailureKeepsSocketOpen pins D-23's retry loop: a rejected join does
// not consume the connection, so the client fixes the nickname and retries
// without a reload. (Research Pitfall 13: the cap counts REJECTIONS, not
// attempts.)
func TestJoinFailureKeepsSocketOpen(t *testing.T) {
	h, _ := newTestHub(t)

	holder := dialWS(t, h)
	join(t, holder, `{"type":"join","role":"player","nickname":"An"}`)

	conn := dialWS(t, h)
	for attempt := 1; attempt <= 3; attempt++ {
		writeFrame(t, conn, `{"type":"join","role":"player","nickname":"An"}`)
		assertErrorCode(t, readFrame(t, conn), CodeNicknameTaken)
	}

	// Still open, and a valid join succeeds on this very socket.
	hello, _ := join(t, conn, `{"type":"join","role":"player","nickname":"Binh"}`)
	if got := frameString(t, frameData(t, hello), "role"); got != "player" {
		t.Fatalf("hello role = %q, want \"player\" — a retry after a rejection must be served", got)
	}
}

// TestJoinFailureCapClosesConnection pins the DoS guard from D-23: rejected
// joins are capped per connection (a named constant), and the connection that
// reaches the cap is closed rather than left for another round.
func TestJoinFailureCapClosesConnection(t *testing.T) {
	h, _ := newTestHub(t)

	holder := dialWS(t, h)
	join(t, holder, `{"type":"join","role":"player","nickname":"An"}`)

	conn := dialWS(t, h)
	for attempt := 1; attempt <= maxRejectedJoins; attempt++ {
		writeFrame(t, conn, `{"type":"join","role":"player","nickname":"An"}`)
		env, err := readFrameErr(conn)
		if err != nil {
			t.Fatalf("attempt %d of %d: the server closed early (%v); the cap must be reached first",
				attempt, maxRejectedJoins, err)
		}
		assertErrorCode(t, env, CodeNicknameTaken)
	}

	// The cap was reached: the connection must now end instead of answering.
	if env, err := readFrameErr(conn); err == nil {
		t.Fatalf("frame %q after %d rejected joins; want the connection closed (D-23 cap)",
			env.Type, maxRejectedJoins)
	}
}

// TestJoinResolvesByToken pins D-30's ladder end to end: a valid token resumes
// the same identity and budget, an unknown token with no nickname is
// token_invalid, and an unknown token with a nickname is a fresh identity whose
// handshake replaces the stale token.
func TestJoinResolvesByToken(t *testing.T) {
	h, engine := newTestHub(t)

	first := dialWS(t, h)
	hello, snapshot := join(t, first, `{"type":"join","role":"player","nickname":"An"}`)
	owner := frameString(t, frameData(t, hello), "player")
	token := frameString(t, frameData(t, hello), "token")
	wantMe := meOf(t, snapshot)

	t.Run("a valid token resumes the same identity and budget", func(t *testing.T) {
		resumed := dialWS(t, h)
		resumedHello, resumedSnapshot := join(t, resumed,
			fmt.Sprintf(`{"type":"join","role":"player","token":%q}`, token))

		if got := frameString(t, frameData(t, resumedHello), "player"); got != owner {
			t.Errorf("resumed player = %q, want %q (D-30 (1): the token wins)", got, owner)
		}
		if got := frameString(t, frameData(t, resumedHello), "token"); got != token {
			t.Errorf("resumed token = %q, want the original %q", got, token)
		}
		if got := meOf(t, resumedSnapshot); got != wantMe {
			t.Errorf("resumed me = %+v, want %+v (the resume restores identity and budget)", got, wantMe)
		}
		if got := engine.PlayerCount(); got != 1 {
			t.Errorf("player count = %d after a resume, want 1 (a resume creates nothing)", got)
		}
	})

	t.Run("an unknown token with no nickname is token_invalid", func(t *testing.T) {
		stranger := dialWS(t, h)
		writeFrame(t, stranger, `{"type":"join","role":"player","token":"deadbeef"}`)
		assertErrorCode(t, readFrame(t, stranger), CodeTokenInvalid)
	})

	t.Run("an unknown token with a nickname joins fresh under a new token", func(t *testing.T) {
		fresh := dialWS(t, h)
		freshHello, _ := join(t, fresh, `{"type":"join","role":"player","nickname":"Binh","token":"deadbeef"}`)
		data := frameData(t, freshHello)
		if got := frameString(t, data, "player"); got == owner {
			t.Error("a stale token resumed the owner's identity (D-30 (2): the nickname wins)")
		}
		if got := frameString(t, data, "token"); got == "deadbeef" {
			t.Error("the handshake echoed the stale token instead of minting a new one")
		}
		if got := engine.PlayerCount(); got != 2 {
			t.Errorf("player count = %d, want 2", got)
		}
	})
}

// TestPreJoinCommand pins D-23's guard: a command that is not a join is
// answered with not_joined, and the socket stays open for the client to join.
func TestPreJoinCommand(t *testing.T) {
	h, _ := newTestHub(t)
	conn := dialWS(t, h)

	writeFrame(t, conn, `{"type":"bid.place","amount":1000}`)
	assertErrorCode(t, readFrame(t, conn), CodeNotJoined)

	writeFrame(t, conn, `{"type":"join","role":"player","nickname":"An"}`)
	if got := readFrame(t, conn).Type; got != TypeHello {
		t.Fatalf("frame after the refused command = %q, want %q — the socket must stay open", got, TypeHello)
	}
}

// TestUnsupportedCommand pins D-24's forward-compatibility rule: a command from
// a later phase is already in the frozen catalog, so it answers the typed
// unsupported error instead of looking like an unknown message.
func TestUnsupportedCommand(t *testing.T) {
	h, _ := newTestHub(t)
	conn := dialWS(t, h)
	join(t, conn, `{"type":"join","role":"admin"}`)

	writeFrame(t, conn, `{"type":"admin.item.start","id":"item-1"}`)
	assertErrorCode(t, readFrame(t, conn), CodeUnsupported)
}

// TestBadMessage pins the two malformed-input paths: a frame that is not JSON
// is answered with the typed bad_message error, and a frame over D-40's 4 KiB
// limit terminates the peer instead of buffering it (T-02-05).
func TestBadMessage(t *testing.T) {
	t.Run("a frame that is not JSON is answered with bad_message", func(t *testing.T) {
		h, _ := newTestHub(t)
		conn := dialWS(t, h)
		writeFrame(t, conn, "this is not json")

		env, err := readFrameErr(conn)
		if err != nil {
			t.Fatalf("no frame after a malformed message: %v (D-24 wants error{code:bad_message} on the same socket)", err)
		}
		assertErrorCode(t, env, "bad_message")
	})

	t.Run("a frame over the 4 KiB limit terminates the peer", func(t *testing.T) {
		h, _ := newTestHub(t)
		conn := dialWS(t, h)

		// Valid JSON, so the only reason it can be refused is the read limit.
		oversized := fmt.Sprintf(`{"type":"join","role":"player","pad":%q}`, strings.Repeat("a", 5<<10))

		_ = conn.SetWriteDeadline(time.Now().Add(testIOTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(oversized)); err != nil {
			// The server may already have closed while the frame was in flight;
			// that is termination too. The read below is the real assertion.
			t.Logf("oversized write errored (%v) — the server closed early", err)
		}

		// The contract is "the payload was not buffered and answered" — not a
		// specific close code: gorilla's on-limit behaviour (1009, ErrReadLimit)
		// is an implementation detail this test must not pin.
		if env, err := readFrameErr(conn); err == nil {
			t.Fatalf("the server answered an oversized frame with %q; the D-40 read limit is not in effect", env.Type)
		}
	})
}

// TestOriginRejectionIsLoggedOncePerInterval pins the abuse guard (IN-07): a
// cross-origin handshake is rejected with 403 and its log line is collapsed to
// one per interval — /ws is unauthenticated, so a scanner must not be able to
// drive unbounded log writes once the app is published. This is also the one
// test that exercises gorilla's same-origin CheckOrigin: DefaultDialer sends no
// Origin header, so every other test here is blind to it.
func TestOriginRejectionIsLoggedOncePerInterval(t *testing.T) {
	lastOriginRejectLog.Store(0) // this test owns the package-level gate
	defer lastOriginRejectLog.Store(0)

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	h, _ := newTestHub(t)
	srv := newTestServer(t, h)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	for attempt := 1; attempt <= 2; attempt++ {
		conn, resp, err := websocket.DefaultDialer.Dial(url, http.Header{"Origin": []string{"http://evil.example"}})
		if conn != nil {
			_ = conn.Close()
		}
		if err == nil {
			t.Fatalf("attempt %d: cross-origin upgrade succeeded; CheckOrigin is not armed", attempt)
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			status := 0
			if resp != nil {
				status = resp.StatusCode
				_ = resp.Body.Close()
			}
			t.Fatalf("attempt %d: HTTP status = %d, want %d", attempt, status, http.StatusForbidden)
		}
		_ = resp.Body.Close()
	}

	// The log call happens in the server handler goroutine, after the 403
	// reaches this one: wait for the first line, then settle briefly so a
	// second (buggy) line has time to land before counting.
	deadline := time.Now().Add(2 * time.Second)
	for buf.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	if got := strings.Count(buf.String(), "ws: upgrade failed"); got != 1 {
		t.Errorf("logged %d upgrade failures for 2 cross-origin attempts, want 1:\n%s", got, buf.String())
	}
}
