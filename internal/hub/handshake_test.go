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
			if role == RoleAdmin {
				// ADMN-04/D-76: the admin role is authorized, and the
				// script/probe channel is join.key.
				joinRequest += adminKeyField
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
		frame := fmt.Sprintf(`{"type":"join","role":%q}`, role)
		if role == RoleAdmin {
			frame = adminJoinJSON // ADMN-04: the admin role is authorized
		}
		env, _ := join(t, conn, frame)
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
// unsupported error instead of looking like an unknown message. Phase 4 routes
// `admin.game.reset`/`admin.config.update`, so the previous example retires and
// the case moves to Phase 5's sealed-bid vocabulary.
func TestUnsupportedCommand(t *testing.T) {
	h, _ := newTestHub(t)
	conn := dialWS(t, h)
	join(t, conn, adminJoinJSON)

	writeFrame(t, conn, `{"type":"dutch.accept","id":"whatever"}`)
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

// TestAdminJoinAuthorization pins ADMN-04/D-76 at the transport edge: the admin
// role is authorized either by the capability cookie that arrived on the HTTP
// upgrade request (the browser's channel) or by the `key` field of the join
// frame (the script/probe channel), and a join carrying neither — or a wrong
// value — is answered with the frozen not_authorized code and retired by the
// very same per-connection rejection cap every other refused join uses. The
// player and screen roles are not touched by the gate.
//
// The check cannot live at the upgrade: the role only exists after the first
// frame (research Pitfall 3), so this is the contract that keeps an
// unauthenticated socket from ever reaching the admin projection.
func TestAdminJoinAuthorization(t *testing.T) {
	t.Run("a join with no credentials is refused and retired at the cap", func(t *testing.T) {
		h, _ := newTestHub(t)
		conn := dialWS(t, h) // no cookie, no key

		// D-23's cap counts REJECTIONS: the socket stays open so a presenter
		// who mistyped can retry, and the last refusal is followed by a close
		// instead of another error frame.
		for attempt := 1; attempt <= maxRejectedJoins; attempt++ {
			writeFrame(t, conn, `{"type":"join","role":"admin"}`)
			env, err := readFrameErr(conn)
			if err != nil {
				t.Fatalf("attempt %d of %d: the server closed early (%v); the cap must be reached first (D-23/D-76)",
					attempt, maxRejectedJoins, err)
			}
			assertErrorCode(t, env, CodeNotAuthorized)
		}

		if env, err := readFrameErr(conn); err == nil {
			t.Fatalf("frame %q after %d refused admin joins; want the connection closed (D-23 cap)",
				env.Type, maxRejectedJoins)
		}
	})

	t.Run("a wrong cookie and a wrong key are both refused", func(t *testing.T) {
		h, _ := newTestHub(t)

		wrongCookie := dialWSWithCookie(t, h, "not-the-capability")
		writeFrame(t, wrongCookie, `{"type":"join","role":"admin"}`)
		assertErrorCode(t, readFrame(t, wrongCookie), CodeNotAuthorized)

		wrongKey := dialWS(t, h)
		writeFrame(t, wrongKey, `{"type":"join","role":"admin","key":"not-the-capability"}`)
		assertErrorCode(t, readFrame(t, wrongKey), CodeNotAuthorized)
	})

	t.Run("the upgrade cookie authorizes the admin handshake", func(t *testing.T) {
		h, _ := newTestHub(t)

		conn := dialWSWithCookie(t, h, testAdminKey)
		hello, snapshot := join(t, conn, `{"type":"join","role":"admin"}`)

		if got := frameString(t, frameData(t, hello), "role"); got != RoleAdmin {
			t.Errorf("hello role = %q, want %q", got, RoleAdmin)
		}
		// AdminView is the secret-capable projection (D-25/D-26), so its
		// presence is the proof that the authorized join reached the admin role
		// rather than some unauthenticated fallback.
		if _, ok := frameData(t, snapshot)["sealed_bids"]; !ok {
			t.Errorf("the admin snapshot carries no sealed_bids block; the cookie-authorized join did not receive the admin projection: %s", snapshot.Data)
		}
	})

	t.Run("join.key authorizes the admin handshake", func(t *testing.T) {
		h, _ := newTestHub(t)

		conn := dialWS(t, h) // no cookie: the script/probe channel
		hello, snapshot := join(t, conn, adminJoinJSON)

		if got := frameString(t, frameData(t, hello), "role"); got != RoleAdmin {
			t.Errorf("hello role = %q, want %q", got, RoleAdmin)
		}
		if _, ok := frameData(t, snapshot)["sealed_bids"]; !ok {
			t.Errorf("the admin snapshot carries no sealed_bids block; the key-authorized join did not receive the admin projection: %s", snapshot.Data)
		}
	})

	t.Run("the player and screen roles stay unauthenticated", func(t *testing.T) {
		h, _ := newTestHub(t)

		player := dialWS(t, h) // no cookie, no key: the classroom join
		if env, _ := join(t, player, `{"type":"join","role":"player","nickname":"An"}`); frameString(t, frameData(t, env), "role") != RolePlayer {
			t.Errorf("player hello role = %q, want %q", frameString(t, frameData(t, env), "role"), RolePlayer)
		}

		screen := dialWS(t, h)
		if env, _ := join(t, screen, `{"type":"join","role":"screen"}`); frameString(t, frameData(t, env), "role") != RoleScreen {
			t.Errorf("screen hello role = %q, want %q", frameString(t, frameData(t, env), "role"), RoleScreen)
		}
	})
}

// TestAdminReset pins D-78/ADMN-03 at the transport edge: `admin.game.reset` is
// routed for an authorized admin only, `keep_players` returns a played room to
// LOBBY through one revision without touching a socket, and `clear_players`
// deletes the room and then closes every player connection with the dedicated
// session-ended code — after the commit, never before it, and never with the
// supersede code a phone would read as "another device took over".
func TestAdminReset(t *testing.T) {
	t.Run("keep_players returns the room to LOBBY and keeps every socket", func(t *testing.T) {
		h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

		admin := joinRole(t, h, RoleAdmin)
		itemID := createItem(t, admin)

		player, _ := joinPlayer(t, h, "An")
		expectAll(t, []*websocket.Conn{admin}, TypeStateSnapshot)

		startItem(t, admin, itemID)
		expectAll(t, []*websocket.Conn{admin, player}, TypePhaseChanged, TypeItemIntro, TypeStateSnapshot)
		startItem(t, admin, itemID)
		expectAll(t, []*websocket.Conn{admin, player}, TypePhaseChanged, TypeItemLive, TypeStateSnapshot, TypePriceTick)
		placeBid(t, player, 100_000)
		expectAll(t, []*websocket.Conn{admin, player}, TypeBidAccepted, TypePriceTick)

		rev := engine.Rev()

		writeFrame(t, admin, `{"type":"admin.game.reset","mode":"keep_players"}`)
		reset := expectAll(t, []*websocket.Conn{admin, player}, TypePhaseChanged, TypeStateSnapshot)

		// One mutation, one revision, both frames on it (D-57).
		if got := reset[0][0].Rev; got != rev+1 {
			t.Errorf("the reset's revision = %d, want exactly one past the accepted bid's %d", got, rev)
		}
		if got := frameString(t, frameData(t, reset[0][0]), "phase"); got != "LOBBY" {
			t.Errorf("phase.changed phase = %q, want LOBBY (D-78)", got)
		}
		if got := frameString(t, frameData(t, reset[0][1]), "phase"); got != "LOBBY" {
			t.Errorf("the reset snapshot's phase = %q, want LOBBY (the snapshot is self-sufficient)", got)
		}
		if raw := dataField(t, reset[0][1], "item"); string(raw) != "null" {
			t.Errorf("the reset snapshot's item = %s, want an explicit null (current_item is cleared)", raw)
		}

		// The item CONFIGURATION survives: the rows are the presenter's setup.
		items := itemsOf(t, reset[0][1])
		if len(items) != 1 || items[0].ID != itemID {
			t.Fatalf("items after a keep_players reset = %+v, want the one configured item", items)
		}
		if items[0].Status != "pending" {
			t.Errorf("item status = %q after the reset, want pending (the round must be playable again)", items[0].Status)
		}

		// Budgets are restored for every identity, and the trail is gone.
		players := playersOf(t, reset[0][1])
		if len(players) != 1 || players[0].Nickname != "An" {
			t.Fatalf("players after a keep_players reset = %+v, want An kept", players)
		}
		if players[0].Budget != 1_000_000 {
			t.Errorf("the kept player's budget = %d, want the starting budget 1000000 (D-78)", players[0].Budget)
		}

		// keep_players closes NOTHING: the same socket answers a resync with a
		// fresh snapshot (a closed socket would fail the read).
		writeFrame(t, player, `{"type":"resync"}`)
		me := meOf(t, readType(t, player, TypeStateSnapshot))
		if me.Budget != 1_000_000 {
			t.Errorf("the kept player's own budget = %d after the reset, want 1000000", me.Budget)
		}
	})

	t.Run("a no-op keep_players reset answers the admin with its own snapshot", func(t *testing.T) {
		h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

		admin := joinRole(t, h, RoleAdmin)

		rev := engine.Rev()

		// A fresh lobby has nothing for keep_players to repair, so the engine
		// commits nothing and fans nothing out. The presenter's panel derives
		// its in-flight flag from its snapshot generation, so the transport edge
		// has to answer the command's own connection — otherwise both Reset
		// buttons stay disabled until some unrelated snapshot arrives.
		writeFrame(t, admin, `{"type":"admin.game.reset","mode":"keep_players"}`)

		confirm := expectFrames(t, admin, TypeStateSnapshot)[0]
		if confirm.Rev != rev {
			t.Errorf("the no-op confirmation carries revision %d, want the unchanged %d (a no-op is not a state event, D-35)", confirm.Rev, rev)
		}
		if got := frameString(t, frameData(t, confirm), "phase"); got != "LOBBY" {
			t.Errorf("the no-op confirmation's phase = %q, want LOBBY", got)
		}
		if got := engine.Rev(); got != rev {
			t.Errorf("revision = %d after a no-op reset, want the unchanged %d", got, rev)
		}
	})

	t.Run("clear_players closes every cleared player's socket after the commit", func(t *testing.T) {
		h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

		admin := joinRole(t, h, RoleAdmin)

		an, _ := joinPlayer(t, h, "An")
		binh, _ := joinPlayer(t, h, "Binh")
		expectAll(t, []*websocket.Conn{admin}, TypeStateSnapshot, TypeStateSnapshot)

		writeFrame(t, admin, `{"type":"admin.game.reset","mode":"clear_players"}`)
		expectAll(t, []*websocket.Conn{admin}, TypePhaseChanged, TypeStateSnapshot)

		// The room is gone from the authoritative state, and both devices were
		// told why with the dedicated code — never 4001, which a player reads as
		// "another device took my identity".
		if got := engine.PlayerCount(); got != 0 {
			t.Errorf("player count = %d after clear_players, want 0", got)
		}
		if code := readCloseCode(t, an); code != 4002 {
			t.Errorf("the cleared player observed close code %d, want 4002 (D-78's deliberate cleanup)", code)
		}
		if code := readCloseCode(t, binh); code != 4002 {
			t.Errorf("the second cleared player observed close code %d, want 4002", code)
		}
	})

	t.Run("an unknown mode is refused without moving the revision", func(t *testing.T) {
		h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

		admin := joinRole(t, h, RoleAdmin)
		joinPlayer(t, h, "An")
		expectAll(t, []*websocket.Conn{admin}, TypeStateSnapshot)

		rev := engine.Rev()

		writeFrame(t, admin, `{"type":"admin.game.reset","mode":"wipe_everything"}`)
		assertErrorCode(t, readFrame(t, admin), CodeBadMessage)

		if got := engine.Rev(); got != rev {
			t.Errorf("revision = %d after a refused mode, want the unchanged %d (a refusal is not a mutation)", got, rev)
		}
		if got := engine.PlayerCount(); got != 1 {
			t.Errorf("player count = %d after a refused mode, want the untouched 1", got)
		}
	})

	t.Run("the reset is refused for every other role", func(t *testing.T) {
		h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

		screen := joinRole(t, h, RoleScreen)
		rev := engine.Rev()

		writeFrame(t, screen, `{"type":"admin.game.reset","mode":"clear_players"}`)
		assertErrorCode(t, readFrame(t, screen), CodeNotAuthorized)

		if got := engine.Rev(); got != rev {
			t.Errorf("revision = %d after a role-refused reset, want the unchanged %d", got, rev)
		}
	})
}

// TestAdminRelease pins D-80 at the transport edge: `admin.player.release`
// names one identity, writes it released, fans the admin's snapshot out on the
// ordinary path, and closes THAT player's socket — with the session-ended code,
// leaving every other connection alone. The released token is dead afterwards,
// which is the whole point of the presenter's insurance (D-30 (1) is gone for
// that row).
func TestAdminRelease(t *testing.T) {
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(64))

	admin := joinRole(t, h, RoleAdmin)

	an := dialWS(t, h)
	anHello, _ := join(t, an, `{"type":"join","role":"player","nickname":"An"}`)
	anID := frameString(t, frameData(t, anHello), "player")
	anToken := frameString(t, frameData(t, anHello), "token")

	binh := dialWS(t, h)
	join(t, binh, `{"type":"join","role":"player","nickname":"Binh"}`)

	expectAll(t, []*websocket.Conn{admin}, TypeStateSnapshot, TypeStateSnapshot)

	writeFrame(t, admin, fmt.Sprintf(`{"type":"admin.player.release","id":%q}`, anID))
	released := expectFrames(t, admin, TypeStateSnapshot)

	// The row survives — a release is not a delete — and the admin's own
	// projection reports it disconnected from the same fan-out as everything
	// else (D-80).
	if got := engine.PlayerCount(); got != 2 {
		t.Errorf("player count = %d after a release, want 2 (the row is kept, only its token is dead)", got)
	}
	rows := playersOf(t, released[0])
	if len(rows) != 2 {
		t.Fatalf("the released snapshot carries %d players, want 2", len(rows))
	}
	for _, row := range rows {
		if row.Nickname == "An" && row.Connected {
			t.Error("the released player still reads connected in the admin view (D-80)")
		}
		if row.Nickname == "An" && row.Budget != 1_000_000 {
			t.Errorf("the released row's budget = %d, want the row kept at 1000000 (a claim preserves it)", row.Budget)
		}
	}

	// The released identity's socket ends with the dedicated code...
	if code := readCloseCode(t, an); code != 4002 {
		t.Errorf("the released player observed close code %d, want 4002 (not the supersede code 4001)", code)
	}

	// ...and the other player's socket is untouched.
	writeFrame(t, binh, `{"type":"resync"}`)
	if got := readType(t, binh, TypeStateSnapshot).Type; got != TypeStateSnapshot {
		t.Errorf("the untouched player received %q, want its own snapshot", got)
	}

	// The stale token can never resume: token lookups skip released rows, so the
	// device that holds it is a stranger (D-30 (1) is gone for this row).
	stranger := dialWS(t, h)
	writeFrame(t, stranger, fmt.Sprintf(`{"type":"join","role":"player","token":%q}`, anToken))
	assertErrorCode(t, readFrame(t, stranger), CodeTokenInvalid)

	// An empty id is a frame-shape failure, and a refusal changes nothing.
	writeFrame(t, admin, `{"type":"admin.player.release"}`)
	assertErrorCode(t, readFrame(t, admin), CodeBadMessage)

	// A re-release of the same row is D-80's no-op: the row already holds the
	// sentinel, so there is no write and no fan-out. The command still answers
	// the presenter's own connection, because its in-flight flag waits for a
	// snapshot and a released row stays in the Players tab until it is claimed.
	writeFrame(t, admin, fmt.Sprintf(`{"type":"admin.player.release","id":%q}`, anID))
	confirm := expectFrames(t, admin, TypeStateSnapshot)[0]
	if rows := playersOf(t, confirm); len(rows) != 2 {
		t.Errorf("the no-op release confirmation carries %d player(s), want the unchanged 2", len(rows))
	}
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
