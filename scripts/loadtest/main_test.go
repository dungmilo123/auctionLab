package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The harness's contract has two halves and both are pinned here: the pure
// stream rules (`stream.observe` / `stream.complete`) and the socket loop
// (`runBot`) against a fake server that speaks the frozen protocol.
//
// The negative cases matter as much as the positive ones. A harness that cannot
// fail would certify a broken server: a seq gap, a duplicate seq, a regressed
// revision and an error frame must all be reported, because each of them is
// exactly what the phase gate uses this binary to detect (D-43).

// helloJSON is D-23's handshake answer for the player the harness joins as.
const helloJSON = `{"role":"player","player":"p1","nickname":"lt-0","token":"0123456789abcdef","rev":1}`

// onePlayerJSON is a lobby snapshot that shows only the bot itself.
const onePlayerJSON = `{"phase":"LOBBY","revision":1,"server_ts":1757934000000,` +
	`"me":{"id":"p1","nickname":"lt-0","budget":1000000,"total_value":0,"connected":true},` +
	`"lobby":[{"nickname":"lt-0","connected":true}],` +
	`"leaderboard":[],"item":null}`

// twoPlayersJSON is the snapshot that proves the other bot is visible.
const twoPlayersJSON = `{"phase":"LOBBY","revision":2,"server_ts":1757934001000,` +
	`"me":{"id":"p1","nickname":"lt-0","budget":1000000,"total_value":0,"connected":true},` +
	`"lobby":[{"nickname":"lt-0","connected":true},{"nickname":"lt-1","connected":true}],` +
	`"leaderboard":[],"item":null}`

// errorJSON is a typed rejection (D-24). A bot that receives one has not joined.
const errorJSON = `{"code":"nickname_taken","message":"that nickname is already taken"}`

// wireJSON builds one frame from the frozen five-field envelope as a string, so
// the fake server can hand raw bytes to the harness.
func wireJSON(seq, rev int64, typ, data string) string {
	return fmt.Sprintf(`{"type":%q,"seq":%d,"rev":%d,"ts":%d,"data":%s}`,
		typ, seq, rev, time.Now().UnixMilli(), data)
}

// wire builds the same frame as a decoded envelope, for the pure-rule tests.
func wire(t *testing.T, seq, rev int64, typ, data string) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal([]byte(wireJSON(seq, rev, typ, data)), &env); err != nil {
		t.Fatalf("building the %s frame: %v", typ, err)
	}
	return env
}

func TestStreamAcceptsThePlayerHandshake(t *testing.T) {
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
		t.Fatalf("the handshake snapshot at seq 2: %v", err)
	}
	if err := s.observe(wire(t, 3, 2, "state.snapshot", twoPlayersJSON)); err != nil {
		t.Fatalf("the next state event at seq 3: %v", err)
	}
	if !s.complete([]string{"lt-0", "lt-1"}) {
		t.Error("the lobby showed both players but the stream did not report complete")
	}
}

func TestStreamRejectsSeqGap(t *testing.T) {
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	err := s.observe(wire(t, 3, 1, "state.snapshot", onePlayerJSON))
	if err == nil {
		t.Fatal("a frame at seq 3 after seq 1 was accepted — a dropped message must fail the run (D-21)")
	}
	if !strings.Contains(err.Error(), "gap") {
		t.Errorf("error = %q, want it to name the seq gap", err)
	}
}

func TestStreamRejectsDuplicateSeq(t *testing.T) {
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	err := s.observe(wire(t, 1, 1, "hello", helloJSON))
	if err == nil {
		t.Fatal("seq 1 arriving twice was accepted — a duplicate must fail the run (D-21)")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("error = %q, want it to name the duplicate", err)
	}
}

func TestStreamRejectsNoncontiguousRevision(t *testing.T) {
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
		t.Fatalf("the handshake snapshot at seq 2: %v", err)
	}
	err := s.observe(wire(t, 3, 4, "state.snapshot", twoPlayersJSON))
	if err == nil {
		t.Fatal("rev 1 followed by rev 4 was accepted — a skipped revision must fail the run (D-22)")
	}
	if !strings.Contains(err.Error(), "rev 4") {
		t.Errorf("error = %q, want it to name the unexpected revision", err)
	}
}

func TestStreamRejectsAnErrorFrame(t *testing.T) {
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	err := s.observe(wire(t, 2, 1, "error", errorJSON))
	if err == nil {
		t.Fatal("an error frame was accepted — a rejected join must fail the run")
	}
	if !strings.Contains(err.Error(), "nickname_taken") {
		t.Errorf("error = %q, want it to name the rejection code", err)
	}
}

func TestStreamRejectsAMalformedEnvelope(t *testing.T) {
	if err := newStream().observe(envelope{Seq: 1, Rev: 1, TS: 1}); err == nil {
		t.Error("a frame with no type was accepted (D-21)")
	}
	if err := newStream().observe(envelope{Type: "hello", Seq: 1, Rev: 1}); err == nil {
		t.Error("a frame with ts 0 was accepted, want a positive server clock (D-21)")
	}
}

func TestStreamCompletesOnlyWhenEveryPeerIsVisible(t *testing.T) {
	peers := []string{"lt-0", "lt-1"}
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
		t.Fatalf("the handshake snapshot at seq 2: %v", err)
	}
	if s.complete(peers) {
		t.Fatal("the stream reported complete while the other player was still unseen")
	}
	if err := s.observe(wire(t, 3, 2, "state.snapshot", twoPlayersJSON)); err != nil {
		t.Fatalf("the state event that reveals the other player: %v", err)
	}
	if !s.complete(peers) {
		t.Error("both players were visible but the stream did not report complete")
	}
}

func TestRunBotReportsDialFailure(t *testing.T) {
	// A real address that nothing listens on: bind a listener and close it, so
	// the failure is a refused connection rather than a DNS error.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := wsURL(dead)
	dead.Close()

	res := runBot(url, botSpec{index: 0, nickname: "lt-0", peers: []string{"lt-0", "lt-1"}})
	if res.ok() {
		t.Fatalf("a dial against a dead endpoint reported success: %+v", res)
	}
}

func TestRunBotSucceedsAgainstTheFrozenHandshake(t *testing.T) {
	server := fakeServer(t, []string{
		wireJSON(1, 1, "hello", helloJSON),
		wireJSON(2, 1, "state.snapshot", onePlayerJSON),
		wireJSON(3, 2, "state.snapshot", twoPlayersJSON),
	})

	res := runBot(wsURL(server), botSpec{index: 0, nickname: "lt-0", peers: []string{"lt-0", "lt-1"}})
	if !res.ok() {
		t.Fatalf("runBot reported failure against a healthy server: %v", res.errs)
	}
	if res.seen != 3 {
		t.Errorf("frames read = %d, want the whole handshake plus the reveal (3)", res.seen)
	}
	if res.rev != 2 {
		t.Errorf("last state rev = %d, want 2", res.rev)
	}
}

func TestRunBotReportsAGap(t *testing.T) {
	server := fakeServer(t, []string{
		wireJSON(1, 1, "hello", helloJSON),
		wireJSON(2, 1, "state.snapshot", onePlayerJSON),
		wireJSON(4, 2, "state.snapshot", twoPlayersJSON), // seq 3 never arrived
	})

	res := runBot(wsURL(server), botSpec{index: 0, nickname: "lt-0", peers: []string{"lt-0", "lt-1"}})
	if res.ok() {
		t.Fatalf("a stream with a seq gap reported success: %+v", res)
	}
	if len(res.errs) == 0 || !strings.Contains(res.errs[0], "gap") {
		t.Errorf("errors = %v, want the seq gap named", res.errs)
	}
}

// fakeServer serves one WebSocket connection that writes the given frames as
// soon as the join arrives, then holds the connection open until the client
// closes it — which is what the real hub's read pump does.
func fakeServer(t *testing.T, frames []string) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		if _, _, err := conn.ReadMessage(); err != nil { // the join
			return
		}
		for _, frame := range frames {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
				return
			}
		}
		_, _, _ = conn.ReadMessage() // block until the bot closes
	}))
	t.Cleanup(server.Close)
	return server
}

// wsURL converts an httptest server's http URL into the ws URL the harness dials.
func wsURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

// --- Phase 3: the auction round's frame contract ---------------------------
//
// These cases pin what `stream.observe` must accept and what it must reject once
// the harness plays a real English round: ticks repeat the current revision
// (D-58), state events advance it contiguously (D-22), and a malformed auction
// payload — a negative remaining window, an unknown rejection reason, a live
// item without its price — fails the run instead of being waved through.
//
// The fixtures are hand-written literals, not the server's own encoder, so a
// wire change has to be a deliberate edit here (the same rule the hub's tests
// follow).

// phaseChangedJSON nests the item object the way D-57's payload does.
func phaseChangedJSON(phase, item string) string {
	return fmt.Sprintf(`{"phase":%q,"item":%s}`, phase, item)
}

// introItemJSON, liveItemJSON and settledItemJSON are the three item shapes a
// round's frames carry.
const (
	introItemJSON = `{"id":"it-1","name":"lt lot","auction_type":"english","start_price":100000,"status":"intro"}`
	liveItemJSON  = `{"id":"it-1","name":"lt lot","auction_type":"english","start_price":100000,"status":"live",` +
		`"current_price":100000,"min_next_bid":100000,"deadline_at":1757934060000,"extensions_used":0}`
	settledItemJSON = `{"id":"it-1","name":"lt lot","auction_type":"english","start_price":100000,"status":"settled",` +
		`"current_price":100000,"deadline_at":1757934060000,"extensions_used":0,` +
		`"settle":{"outcome":"sold","winner":{"nickname":"lt-a"},"price":100000,"truth_value":350000,"budget_delta":250000}}`
	tickJSON = `{"item_id":"it-1","current_price":100000,"min_next_bid":110000,` +
		`"deadline_at":1757934060000,"remaining_ms":45000}`
	acceptedJSON = `{"player_id":"p-a","nickname":"lt-a","amount":100000,"current_price":100000,` +
		`"min_next_bid":110000,"deadline_at":1757934060000,"extensions_used":0}`
	rejectedJSON    = `{"reason":"below_minimum","min_next_bid":110000,"budget":1000000,"amount":100000}`
	leaderboardJSON = `[{"rank":1,"nickname":"lt-a","total_value":350000,"budget":1250000}]`
)

// TestStreamAcceptsTheAuctionRoundFrames walks one round's frames in order: the
// phase and item events of a mutation share its revision, a tick repeats that
// revision without advancing it, the private rejection is not a state event, and
// the settle trio closes the round at one more revision.
func TestStreamAcceptsTheAuctionRoundFrames(t *testing.T) {
	s := newStream()

	frames := []struct {
		seq, rev int64
		typ      string
		data     string
	}{
		{1, 1, "hello", helloJSON},
		{2, 1, "state.snapshot", twoPlayersJSON},
		{3, 2, "phase.changed", phaseChangedJSON("ITEM_INTRO", introItemJSON)},
		{4, 2, "item.intro", introItemJSON},
		{5, 2, "state.snapshot", twoPlayersJSON},
		{6, 3, "phase.changed", phaseChangedJSON("LIVE", liveItemJSON)},
		{7, 3, "item.live", liveItemJSON},
		{8, 3, "state.snapshot", twoPlayersJSON},
		{9, 3, "price.tick", tickJSON},
		{10, 4, "bid.accepted", acceptedJSON},
		{11, 4, "price.tick", tickJSON},
		{12, 4, "bid.rejected", rejectedJSON}, // private: no revision movement
		{13, 5, "item.settled", settledItemJSON},
		{14, 5, "leaderboard", leaderboardJSON},
		{15, 5, "state.snapshot", twoPlayersJSON},
	}
	for i, want := range frames {
		if err := s.observe(wire(t, want.seq, want.rev, want.typ, want.data)); err != nil {
			t.Fatalf("frame %d (%s at rev %d): %v", i, want.typ, want.rev, err)
		}
	}
	if s.lastRev != 5 {
		t.Errorf("last state revision = %d, want 5 — ticks and rejections must not move it (D-58)", s.lastRev)
	}
	if s.want != 16 {
		t.Errorf("next expected seq = %d, want 16 (every frame was consumed exactly once)", s.want)
	}
}

// TestStreamRejectsMalformedAuctionFrames is the harness's own tooth test: each
// fixture below is a wire shape the server must never send, and a validator that
// accepted it would certify a broken round.
func TestStreamRejectsMalformedAuctionFrames(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		data string
	}{
		{"tick with a negative remaining window", "price.tick",
			`{"item_id":"it-1","current_price":100000,"min_next_bid":110000,"deadline_at":1757934060000,"remaining_ms":-1}`},
		{"tick whose remaining window has no deadline to anchor on", "price.tick",
			`{"item_id":"it-1","current_price":100000,"min_next_bid":110000,"remaining_ms":45000}`},
		{"tick with no current price", "price.tick",
			`{"item_id":"it-1","min_next_bid":110000,"deadline_at":1757934060000,"remaining_ms":45000}`},
		{"rejection with an unknown reason", "bid.rejected",
			`{"reason":"too_low","min_next_bid":110000,"budget":1000000,"amount":100000}`},
		{"live item with no price", "item.live",
			`{"id":"it-1","name":"lt lot","auction_type":"english","start_price":100000,"status":"live","min_next_bid":100000,"deadline_at":1757934060000}`},
		{"settled item with no reveal", "item.settled",
			`{"id":"it-1","name":"lt lot","auction_type":"english","start_price":100000,"status":"settled"}`},
		{"accepted bid with no amount", "bid.accepted",
			`{"player_id":"p-a","nickname":"lt-a","current_price":100000,"min_next_bid":110000}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newStream()
			if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
				t.Fatalf("hello: %v", err)
			}
			if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
				t.Fatalf("handshake snapshot: %v", err)
			}
			if err := s.observe(wire(t, 3, 2, tc.typ, tc.data)); err == nil {
				t.Fatalf("a %s frame with %s was accepted", tc.typ, tc.name)
			}
		})
	}
}

// TestStreamIsRoleAware pins the role-specific halves of the handshake and the
// snapshot: the admin and the screen join without an identity, so their hello
// carries no player id or token and their snapshots carry no `me` block — and a
// player's stream still requires both.
func TestStreamIsRoleAware(t *testing.T) {
	admin := newStreamAs("admin")
	if err := admin.observe(wire(t, 1, 1, "hello", `{"role":"admin","rev":1}`)); err != nil {
		t.Fatalf("admin hello: %v", err)
	}
	if err := admin.observe(wire(t, 2, 1, "state.snapshot", `{"phase":"LOBBY","revision":1,"server_ts":1,"items":[],"lobby":[],"leaderboard":[],"item":null}`)); err != nil {
		t.Fatalf("admin snapshot: %v", err)
	}

	player := newStreamAs("player")
	err := player.observe(wire(t, 1, 1, "hello", `{"role":"admin","rev":1}`))
	if err == nil {
		t.Fatal("a player stream accepted an admin hello")
	}
}

// TestItemAndSettleDecoders pins the two readers the round's assertions use:
// the item object is nested under `item` for phase.changed and direct for the
// item events, and the reveal is either present (with a null winner for an
// unsold item) or absent — never a null object (D-67's boundary).
func TestItemAndSettleDecoders(t *testing.T) {
	nested, err := itemObj("phase.changed", json.RawMessage(phaseChangedJSON("LIVE", liveItemJSON)))
	if err != nil {
		t.Fatalf("phase.changed item: %v", err)
	}
	if got, _ := stringKey(nested, "status"); got != "live" {
		t.Errorf("phase.changed item status = %q, want live", got)
	}

	direct, err := itemObj("item.settled", json.RawMessage(settledItemJSON))
	if err != nil {
		t.Fatalf("item.settled item: %v", err)
	}
	reveal, err := settleOf(direct)
	if err != nil {
		t.Fatalf("settleOf(sold): %v", err)
	}
	if reveal == nil || reveal.Outcome != "sold" || reveal.Price != 100000 {
		t.Fatalf("settleOf(sold) = %+v, want the sold reveal", reveal)
	}
	if reveal.Winner == nil || reveal.Winner.Nickname != "lt-a" {
		t.Errorf("settleOf(sold) winner = %+v, want lt-a", reveal.Winner)
	}
	if reveal.TruthValue != 350000 || reveal.BudgetDelta != 250000 {
		t.Errorf("settleOf(sold) = %+v, want truth_value 350000 and delta 250000", reveal)
	}

	unsold, err := settleOf(map[string]json.RawMessage{
		"status": json.RawMessage(`"settled"`),
		"settle": json.RawMessage(`{"outcome":"unsold","winner":null,"price":0,"truth_value":350000,"budget_delta":0}`),
	})
	if err != nil {
		t.Fatalf("settleOf(unsold): %v", err)
	}
	if unsold == nil || unsold.Outcome != "unsold" || unsold.Winner != nil {
		t.Errorf("settleOf(unsold) = %+v, want an explicit null winner", unsold)
	}

	if _, err := settleOf(map[string]json.RawMessage{"status": json.RawMessage(`"live"`)}); err == nil {
		t.Error("settleOf accepted an item with no settle key: the boundary is the object's absence")
	}
}

// TestRoundItemLookupFindsTheCreatedItem pins the admin-side read the round's
// first step depends on: the CRUD snapshot's `items` array carries the new id.
func TestRoundItemLookupFindsTheCreatedItem(t *testing.T) {
	snapshot := `{"phase":"LOBBY","revision":2,"server_ts":1,"config":{"starting_budget":1000000,"currency":"VND"},` +
		`"players":[],"items":[{"id":"it-9","name":"lt lot","auction_type":"english","status":"pending"}],` +
		`"sealed_bids":[],"lobby":[],"leaderboard":[],"item":null}`
	id, err := itemIDFromSnapshot(json.RawMessage(snapshot))
	if err != nil {
		t.Fatalf("itemIDFromSnapshot: %v", err)
	}
	if id != "it-9" {
		t.Errorf("item id = %q, want it-9", id)
	}
	if _, err := itemIDFromSnapshot(json.RawMessage(`{"items":[]}`)); err == nil {
		t.Error("itemIDFromSnapshot accepted a snapshot with no items")
	}
}

// --- Phase 7: the N-bot rapid-fire modes (D-135/D-136) ----------------------
//
// The cases above pin the STRICT contract: with two bots and no backpressure,
// every gap is a failure. A 20-bot rapid-fire run is the one context where a
// game-frame gap is legitimate — and only in the exact shape D-41 guarantees,
// where the frame that reveals the gap IS the recovery snapshot. These cases pin
// that asymmetry, the two control-plane frames D-132/D-123 added (heartbeat,
// tunnel.status), the end-of-run convergence assertion, and the measured-rate
// reporting D-138's bucket sizing is judged from.
//
// A harness that certified a broken run would be worse than no harness, so the
// negative cases here matter as much as the positive ones.

// TestStreamClassifiesControlPlaneFramesWithoutMovingRevision pins the two
// control-plane types: each takes a real seq on the FIFO, moves NO revision, and
// is applied at any seq by the browser gate — so a stale copy is ignored here
// rather than failing a healthy run.
func TestStreamClassifiesControlPlaneFramesWithoutMovingRevision(t *testing.T) {
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
		t.Fatalf("the handshake snapshot at seq 2: %v", err)
	}

	if err := s.observe(wire(t, 3, 1, typeHeartbeat, `{"ts":1757934002000}`)); err != nil {
		t.Fatalf("a heartbeat at seq 3: %v", err)
	}
	if err := s.observe(wire(t, 4, 1, typeTunnelStatus,
		`{"url":"https://control.trycloudflare.com","healthy":true}`)); err != nil {
		t.Fatalf("a tunnel.status at seq 4: %v", err)
	}
	if s.lastRev != 1 {
		t.Errorf("last revision = %d, want the unchanged 1 — a control frame is not a state event (D-132)", s.lastRev)
	}
	if s.want != 5 {
		t.Errorf("next expected seq = %d, want 5 (both control frames took their seq)", s.want)
	}

	// A beat that has already been counted: the browser gate applies it anyway,
	// and the harness must not treat it as a duplicate (D-132's "at any seq").
	if err := s.observe(wire(t, 3, 1, typeHeartbeat, `{"ts":1757934002000}`)); err != nil {
		t.Fatalf("a stale heartbeat: %v", err)
	}
	if s.want != 5 {
		t.Errorf("next expected seq = %d after a stale beat, want the unchanged 5", s.want)
	}

	// The stream continues exactly where it was.
	if err := s.observe(wire(t, 5, 2, "state.snapshot", twoPlayersJSON)); err != nil {
		t.Fatalf("the state event after the control frames: %v", err)
	}
	if s.lastRev != 2 {
		t.Errorf("last revision = %d, want 2 — the stream resumed at the next state event", s.lastRev)
	}
}

// TestStreamRapidFireAcceptsARepairedGap is D-41's one legal gap: in rapid mode
// a game-frame gap is accepted only when the frame revealing it is the recovery
// snapshot, and that snapshot re-bases the revision accounting (a repair
// legitimately jumps many revisions).
func TestStreamRapidFireAcceptsARepairedGap(t *testing.T) {
	s := newRapidStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
		t.Fatalf("the handshake snapshot at seq 2: %v", err)
	}

	// Six revisions were committed while this client was fenced: the frames
	// between seq 2 and seq 9 were suppressed, and the recovery snapshot is
	// what arrives next (D-41).
	if err := s.observe(wire(t, 9, 7, "state.snapshot", twoPlayersJSON)); err != nil {
		t.Fatalf("a gap repaired by the snapshot that reveals it: %v", err)
	}
	if s.lastRev != 7 {
		t.Errorf("last revision = %d, want the repair's 7", s.lastRev)
	}
	if s.gaps != 1 || s.repairs != 1 {
		t.Errorf("gaps/repairs = %d/%d, want 1/1 — a drill must show the repair happened", s.gaps, s.repairs)
	}
	if s.want != 10 {
		t.Errorf("next expected seq = %d, want 10 (the repair took its own seq)", s.want)
	}

	// Normal accounting resumes from the rebased revision.
	if err := s.observe(wire(t, 10, 8, "state.snapshot", twoPlayersJSON)); err != nil {
		t.Fatalf("the state event after the repair: %v", err)
	}
}

// TestStreamRapidFireRejectsASilentGap is D-135's "no silent gap": when a game
// frame arrives past its expected seq and the frame is NOT the recovery
// snapshot, the run fails — the relaxation must never become "anything goes".
func TestStreamRapidFireRejectsASilentGap(t *testing.T) {
	s := newRapidStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
		t.Fatalf("the handshake snapshot at seq 2: %v", err)
	}

	err := s.observe(wire(t, 9, 7, "leaderboard", leaderboardJSON))
	if err == nil {
		t.Fatal("a game frame past its expected seq was accepted without a repair snapshot — that is a silent gap")
	}
	if !strings.Contains(err.Error(), "silent gap") {
		t.Errorf("error = %q, want it to name the silent gap, not just any seq problem", err)
	}
}

// TestStreamStrictSmokeStillRejectsAGap pins the asymmetry the two modes exist
// for: `make loadtest`'s two-bot smoke keeps failing on ANY game-frame gap, so
// raising the pool behind `--bots` never weakens the gate that runs before every
// rehearsal.
func TestStreamStrictSmokeStillRejectsAGap(t *testing.T) {
	s := newStream()
	if err := s.observe(wire(t, 1, 1, "hello", helloJSON)); err != nil {
		t.Fatalf("hello at seq 1: %v", err)
	}
	if err := s.observe(wire(t, 2, 1, "state.snapshot", onePlayerJSON)); err != nil {
		t.Fatalf("the handshake snapshot at seq 2: %v", err)
	}
	// The very shape rapid mode accepts: the smoke must still refuse it.
	if err := s.observe(wire(t, 9, 7, "state.snapshot", twoPlayersJSON)); err == nil {
		t.Fatal("a gap repaired by a snapshot was accepted in strict mode — the smoke path must stay strict (D-135)")
	}
}

// TestConvergenceIsTheEndOfRunAssertion pins D-135's strongest assertion: every
// bot's last state revision must equal the server's final revision, and a single
// lagging bot fails the run with its index named.
func TestConvergenceIsTheEndOfRunAssertion(t *testing.T) {
	if err := converged([]int64{12, 12, 12}, 12); err != nil {
		t.Errorf("converged(three bots at 12, want 12) = %v, want nil", err)
	}

	err := converged([]int64{12, 12, 11}, 12)
	if err == nil {
		t.Fatal("converged() certified a run in which one bot never reached the final revision")
	}
	if !strings.Contains(err.Error(), "bot 2") {
		t.Errorf("error = %q, want it to name the lagging bot", err)
	}
	if err := converged(nil, 12); err == nil {
		t.Error("converged() accepted an empty result set")
	}
}

// TestFrameBudgetScalesWithThePool pins the bound that keeps a 20-bot run from
// reading forever: the two-bot smoke keeps its shipped floor, and a bigger pool
// gets a strictly bigger budget for the fan-out it will see.
func TestFrameBudgetScalesWithThePool(t *testing.T) {
	if got := frameBudget(bots); got < maxFrames {
		t.Errorf("frameBudget(2) = %d, want at least the shipped floor %d", got, maxFrames)
	}
	if got := frameBudget(20); got <= frameBudget(bots) {
		t.Errorf("frameBudget(20) = %d, want it strictly past frameBudget(2) = %d", got, frameBudget(bots))
	}
	if got := frameBudget(0); got < maxFrames {
		t.Errorf("frameBudget(0) = %d, want it clamped to the smoke pool's budget", got)
	}
}

// TestPeakCommandRateIsTheD138Evidence pins the measured rate the acceptance run
// prints: the largest number of commands inside any one-second window, so the
// production bucket (20/s, burst 40) can be judged against what the harness
// actually asked of it.
func TestPeakCommandRateIsTheD138Evidence(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)

	burst := []time.Time{base, base.Add(10 * time.Millisecond), base.Add(20 * time.Millisecond)}
	if got := peakCommandRate(burst); got != 3 {
		t.Errorf("peakCommandRate(3 commands in 20ms) = %v, want 3", got)
	}

	spread := []time.Time{base, base.Add(2 * time.Second), base.Add(4 * time.Second)}
	if got := peakCommandRate(spread); got != 1 {
		t.Errorf("peakCommandRate(1 command per 2s) = %v, want 1", got)
	}

	if got := peakCommandRate(nil); got != 0 {
		t.Errorf("peakCommandRate(nothing) = %v, want 0", got)
	}
}
