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
