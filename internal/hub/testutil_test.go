package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
	"auctionlab/internal/store"
)

// This file is the shared transport fixture for the hub's real tests: a fresh
// engine on a temp database, an httptest server around the hub's WebSocket
// handler, and gorilla's DefaultDialer over the real socket — the Phase 1
// idiom, kept because the contracts below are transport contracts (D-43), not
// mock contracts.
//
// Note for the record: the dialer sends no Origin header, so this idiom cannot
// exercise gorilla's same-origin CheckOrigin; that path is owned by
// TestOriginRejectionIsLoggedOncePerInterval.

// testIOTimeout bounds every read and write: a handler that answers nothing has
// to fail the case instead of hanging the package.
const testIOTimeout = 3 * time.Second

// testAdminKey is ADMN-04's capability for this package's tests. The hub is
// built with it, and every admin handshake below carries it through one of
// D-76's two channels — the join.key field a script/probe uses
// (TestAdminJoinAuthorization) or the `auction_admin` cookie a browser sends on
// the upgrade request (dialWSWithCookie).
//
// The literal is the production SHAPE (64 hex characters, D-74) but no real key:
// a fixture that could accidentally match a live server's capability would be a
// leak, not a test.
const testAdminKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// adminKeyField is D-76's script/probe credential as a JSON fragment, so the
// admin frames in this package cannot drift apart in spelling.
const adminKeyField = `,"key":"` + testAdminKey + `"`

// adminJoinJSON is the complete admin handshake frame: since ADMN-04 the admin
// role is authorized, and every existing admin fixture carries the test key
// through the channel the browser never needs (D-76 keeps the browser on the
// cookie, so production JS never has to read the capability).
const adminJoinJSON = `{"type":"join","role":"admin"` + adminKeyField + `}`

// testPolicy is D-40's shape with values small enough to observe: a 4-deep FIFO
// (so a stalled client overflows deterministically) and liveness timings short
// enough that a pong window is not a test's runtime. The mechanism is what is
// under test; the production numbers live in DefaultPolicy and are pinned
// literally by TestDefaultPolicyPinsTheD40Numbers.
func testPolicy() Policy {
	return testPolicyWithBuffer(4)
}

// testPolicyWithBuffer is testPolicy with the FIFO depth chosen by the case: the
// burst and stalled-client tests need a known depth, the overflow test needs a
// small one.
func testPolicyWithBuffer(depth int) Policy {
	return Policy{
		SendBuffer: depth,
		ReadLimit:  4 << 10,
		PongWait:   2 * time.Second,
		PingPeriod: 200 * time.Millisecond,
		WriteWait:  2 * time.Second,
		// D-56: the mechanism is what the tests pin, never the production
		// cadence — a fast beat keeps the ticker cases well under the read
		// budget instead of sleeping on 250 ms (H16).
		TickPeriod: 10 * time.Millisecond,
	}
}

// testClock is the injectable clock D-38 requires: tests move time instead of
// sleeping, and the SAME instant drives both the engine and the envelope clock.
type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

// newTestHub builds a hub on its own engine and database, with the fan-out edge
// installed exactly as cmd/server installs it.
func newTestHub(t *testing.T) (*Hub, *game.Engine) {
	t.Helper()
	return newTestHubWithPolicy(t, testPolicy())
}

// newTestHubWithPolicy is newTestHub with the D-40 policy injected, for the
// cases that need a deeper FIFO or a different liveness window.
func newTestHubWithPolicy(t *testing.T, policy Policy) (*Hub, *game.Engine) {
	t.Helper()
	return newTestHubAt(t, policy, nil)
}

// newTestHubWithClock pins the engine's injected clock and the hub's envelope
// clock to the same instant, so a ticker test can advance time as data: the
// deadline, the tick's remaining_ms and the envelope ts all move together.
func newTestHubWithClock(t *testing.T, policy Policy, clock *testClock) (*Hub, *game.Engine) {
	t.Helper()
	return newTestHubAt(t, policy, clock)
}

// newTestHubAt is the shared constructor: nil clock means the real one.
func newTestHubAt(t *testing.T, policy Policy, clock *testClock) (*Hub, *game.Engine) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "auction.db"))
	if err != nil {
		t.Fatalf("opening the test database: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := store.Schema(st.DB); err != nil {
		t.Fatalf("applying the schema: %v", err)
	}

	now := time.Now
	if clock != nil {
		now = clock.now
	}
	engine, err := game.New(game.Config{
		Store: st,
		Now:   now,
		// A commit failure is fatal in production; here it fails the test with
		// the engine's own message instead of exiting the test binary.
		Fatal: func(err error) { t.Errorf("engine reported an unrecoverable commit failure: %v", err) },
	})
	if err != nil {
		t.Fatalf("building the engine: %v", err)
	}

	h := newHub(engine, policy, testAdminKey)
	if clock != nil {
		h.now = clock.now
	}
	engine.SetPublisher(h.Fanout) // the boot's fan-out edge (D-33/D-57)
	return h, engine
}

// newTestServer serves the hub's WebSocket handler on a real listener.
//
// httptest does not track a *hijacked* connection as an outstanding request, so
// Server.Close can return while the handler goroutine is still running its
// deferred teardown. That teardown writes presence (D-37), i.e. it commits —
// and a commit against a store this fixture has already closed would trip the
// engine's fatal hook. The wait group below is what makes the shutdown order
// deterministic: the connection is closed first, then the server, then this
// waits for the last handler to return, and only then does the store close.
func newTestServer(t *testing.T, h *Hub) *httptest.Server {
	t.Helper()

	var handlers sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Done()
		h.ServeWS(w, r)
	}))
	t.Cleanup(func() {
		srv.Close()
		handlers.Wait()
	})
	return srv
}

// dialURL dials a running test server. The connection is closed at cleanup.
func dialURL(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()

	return dialURLWithHeader(t, srv, nil)
}

// testAdminCookie is D-75's capability cookie as it reaches this package: the
// name is a wire contract shared with internal/api, written here as a literal on
// purpose (a rename in either package has to break a test, and api's own
// TestAdminGate pins the same spelling at the HTTP edge).
const testAdminCookie = "auction_admin"

// dialURLWithCookie dials with D-75's admin capability cookie on the upgrade
// request — the channel a browser uses. gorilla's Dialer takes request headers,
// which is where a cookie has to travel (RESEARCH §Code Examples, verified
// against the gorilla DialContext docs); without this the same-origin page would
// be the only way to exercise the cookie arm at the socket.
func dialURLWithCookie(t *testing.T, srv *httptest.Server, value string) *websocket.Conn {
	t.Helper()

	return dialURLWithHeader(t, srv, http.Header{
		"Cookie": []string{testAdminCookie + "=" + value},
	})
}

// dialURLWithHeader is the one dial: an extra request header, or none.
func dialURLWithHeader(t *testing.T, srv *httptest.Server, header http.Header) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
		}
		t.Fatalf("dialing %s: %v (HTTP %d)", url, err, status)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// dialWS is the one-connection case: a private server plus a dial.
func dialWS(t *testing.T, h *Hub) *websocket.Conn {
	t.Helper()
	return dialURL(t, newTestServer(t, h))
}

// dialWSWithCookie is dialWS with the admin capability cookie on the request.
func dialWSWithCookie(t *testing.T, h *Hub, value string) *websocket.Conn {
	t.Helper()
	return dialURLWithCookie(t, newTestServer(t, h), value)
}

// frame is one server frame decoded for a contract assertion: the five D-21
// fields plus the raw bytes and the raw key set of the object that actually
// arrived, so a case can assert the wire shape literally instead of trusting a
// Go struct.
type frame struct {
	Type string          `json:"type"`
	Seq  int64           `json:"seq"`
	Rev  int64           `json:"rev"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data"`

	raw  []byte
	keys []string
}

// decodeFrame turns one wire frame into a frame, keeping the bytes.
func decodeFrame(raw []byte) (frame, error) {
	var env frame
	if err := json.Unmarshal(raw, &env); err != nil {
		return frame{}, fmt.Errorf("frame is not a JSON object: %v (%s)", err, raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return frame{}, fmt.Errorf("frame has no key set: %v (%s)", err, raw)
	}
	for key := range fields {
		env.keys = append(env.keys, key)
	}
	sort.Strings(env.keys)
	env.raw = raw
	return env, nil
}

// writeFrame sends one text frame and fails the case if the socket refuses it.
func writeFrame(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()

	if err := writeFrameErr(conn, payload); err != nil {
		t.Fatalf("writing %s: %v", payload, err)
	}
}

// writeFrameErr is writeFrame without t.Fatalf: a burst case writes from several
// goroutines, where FailNow is not allowed (it would end the wrong goroutine).
func writeFrameErr(conn *websocket.Conn, payload string) error {
	_ = conn.SetWriteDeadline(time.Now().Add(testIOTimeout))
	return conn.WriteMessage(websocket.TextMessage, []byte(payload))
}

// readType reads frames until one of the wanted type arrives, so a case can
// skip the interleaved traffic of another client's fan-out.
func readType(t *testing.T, conn *websocket.Conn, typ string) frame {
	t.Helper()

	deadline := time.Now().Add(testIOTimeout)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no %q frame arrived within %s", typ, testIOTimeout)
		}
		env, err := readFrameErr(conn)
		if err != nil {
			t.Fatalf("waiting for a %q frame: %v", typ, err)
		}
		if env.Type == typ {
			return env
		}
	}
}

// reader drains one socket in its own goroutine. A case that must keep one
// connection healthy while it provokes a stall on another cannot call t.Fatalf
// from that goroutine, so the failures arrive as values.
type reader struct {
	frames chan frame
	errs   chan error
}

// startReader begins draining conn. The socket's own read deadline ends the
// reader when the connection goes quiet for testIOTimeout.
func startReader(conn *websocket.Conn) *reader {
	r := &reader{
		frames: make(chan frame, 512),
		errs:   make(chan error, 1),
	}
	go func() {
		for {
			env, err := readFrameErr(conn)
			if err != nil {
				select {
				case r.errs <- err:
				default:
				}
				close(r.frames)
				return
			}
			r.frames <- env
		}
	}()
	return r
}

// next returns the next frame the reader saw, or fails the case if none arrived.
func (r *reader) next(t *testing.T) frame {
	t.Helper()

	select {
	case env, ok := <-r.frames:
		if !ok {
			t.Fatal("the connection ended before the expected frame arrived")
		}
		return env
	case <-time.After(testIOTimeout):
		t.Fatal("no frame arrived within the read budget")
		return frame{}
	}
}

// readFrame reads the next frame and fails the case if none arrives.
func readFrame(t *testing.T, conn *websocket.Conn) frame {
	t.Helper()

	env, err := readFrameErr(conn)
	if err != nil {
		t.Fatalf("reading a frame: %v", err)
	}
	return env
}

// readFrameErr reads the next frame, or reports why none arrived. Separate from
// readFrame so the "the server must stay silent / must close here" paths can
// assert on the error instead of dying on it.
func readFrameErr(conn *websocket.Conn) (frame, error) {
	_ = conn.SetReadDeadline(time.Now().Add(testIOTimeout))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return frame{}, err
	}
	return decodeFrame(raw)
}

// drainFrames reads until the socket has been quiet for quiet and returns
// everything it saw.
//
// TERMINAL: a gorilla read deadline that expires leaves the connection in a
// permanent error state ("errors returned from this method are permanent"), so
// the quiet window this helper waits for also ends the connection. Only call it
// when nothing else needs that socket afterwards.
func drainFrames(t *testing.T, conn *websocket.Conn, quiet time.Duration) []frame {
	t.Helper()

	var frames []frame
	for {
		_ = conn.SetReadDeadline(time.Now().Add(quiet))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return frames
		}
		env, err := decodeFrame(raw)
		if err != nil {
			t.Fatalf("reading a frame: %v", err)
		}
		frames = append(frames, env)
	}
}

// frameData decodes the frame's data object.
func frameData(t *testing.T, env frame) map[string]json.RawMessage {
	t.Helper()

	var data map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("data is not a JSON object: %v (%s)", err, env.Data)
	}
	return data
}

// dataField is one raw field of the frame's data object.
func dataField(t *testing.T, env frame, key string) json.RawMessage {
	t.Helper()

	raw, ok := frameData(t, env)[key]
	if !ok {
		t.Fatalf("data has no %q key: %s", key, env.Data)
	}
	return raw
}

// frameString reads one string field out of a decoded data object.
func frameString(t *testing.T, data map[string]json.RawMessage, key string) string {
	t.Helper()

	raw, ok := data[key]
	if !ok {
		t.Fatalf("data has no %q key: %v", key, data)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("data.%s is not a string: %v (%s)", key, err, raw)
	}
	return value
}

// assertErrorCode pins the typed error contract: error{code:<want>}.
func assertErrorCode(t *testing.T, env frame, want string) {
	t.Helper()

	if env.Type != TypeError {
		t.Fatalf("frame type = %q, want %q (with code %q)", env.Type, TypeError, want)
	}
	if got := frameString(t, frameData(t, env), "code"); got != want {
		t.Fatalf("error code = %q, want %q", got, want)
	}
}

// assertEnvelopeKeys pins the exact key set of a frame. Exact, not a superset:
// D-24's freeze means a renamed or removed field has to be a deliberate edit
// that breaks this test, and D-21 means a frame without seq/rev is not this
// protocol at all.
func assertEnvelopeKeys(t *testing.T, env frame) {
	t.Helper()

	want := []string{"data", "rev", "seq", "ts", "type"}
	if len(env.keys) != len(want) {
		t.Fatalf("envelope keys = %v, want exactly %v (D-21/D-24)", env.keys, want)
	}
	for i, key := range want {
		if env.keys[i] != key {
			t.Fatalf("envelope keys = %v, want exactly %v (D-21/D-24)", env.keys, want)
		}
	}
}

// join performs a clean handshake and returns both frames of the D-23 pair:
// hello first, the role snapshot second. Every case below starts here, because
// the pair's ordering is itself part of the contract.
func join(t *testing.T, conn *websocket.Conn, payload string) (frame, frame) {
	t.Helper()

	writeFrame(t, conn, payload)

	hello := readFrame(t, conn)
	if hello.Type != TypeHello {
		t.Fatalf("first frame after %s = %q, want %q", payload, hello.Type, TypeHello)
	}
	snapshot := readFrame(t, conn)
	if snapshot.Type != TypeStateSnapshot {
		t.Fatalf("second frame after %s = %q, want %q", payload, snapshot.Type, TypeStateSnapshot)
	}
	return hello, snapshot
}

// playerIdentity is the player view's `me` object, as far as the transport
// tests need it: the identity and the budget a resume has to restore (D-30).
type playerIdentity struct {
	ID     string `json:"id"`
	Budget int64  `json:"budget"`
}

// meOf reads a player snapshot's own-identity block.
func meOf(t *testing.T, snapshot frame) playerIdentity {
	t.Helper()

	var me playerIdentity
	if err := json.Unmarshal(dataField(t, snapshot, "me"), &me); err != nil {
		t.Fatalf("snapshot me is not a player identity: %v", err)
	}
	return me
}

// lobbyEntry is one row of the lobby list as it reaches the wire.
type lobbyEntry struct {
	Nickname  string `json:"nickname"`
	Connected bool   `json:"connected"`
}

// adminPlayerRow is one row of the admin view's player list, as the reset and
// release cases read it: the Players tab's fields those mutations have to move.
type adminPlayerRow struct {
	Nickname  string `json:"nickname"`
	Budget    int64  `json:"budget"`
	Connected bool   `json:"connected"`
}

// playersOf decodes an admin snapshot's player list.
func playersOf(t *testing.T, snapshot frame) []adminPlayerRow {
	t.Helper()

	var rows []adminPlayerRow
	if err := json.Unmarshal(dataField(t, snapshot, "players"), &rows); err != nil {
		t.Fatalf("the admin snapshot's players are not a list of player rows: %v", err)
	}
	return rows
}

// itemRow is one row of the admin view's item list: the reset case needs the
// item CONFIG (what a reset must keep) beside its status (what it rewrites).
type itemRow struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// itemsOf decodes an admin snapshot's item list.
func itemsOf(t *testing.T, snapshot frame) []itemRow {
	t.Helper()

	var rows []itemRow
	if err := json.Unmarshal(dataField(t, snapshot, "items"), &rows); err != nil {
		t.Fatalf("the admin snapshot's items are not a list of item rows: %v", err)
	}
	return rows
}

// lobbyOf decodes a snapshot's lobby list.
func lobbyOf(t *testing.T, snapshot frame) []lobbyEntry {
	t.Helper()

	var entries []lobbyEntry
	if err := json.Unmarshal(dataField(t, snapshot, "lobby"), &entries); err != nil {
		t.Fatalf("snapshot lobby is not a list of entries: %v", err)
	}
	return entries
}

// lobbyConnected reports the presence flag a snapshot's lobby shows for one
// nickname — the observable D-37 state the supersede teardown must not flip.
func lobbyConnected(t *testing.T, snapshot frame, nickname string) bool {
	t.Helper()

	for _, entry := range lobbyOf(t, snapshot) {
		if entry.Nickname == nickname {
			return entry.Connected
		}
	}
	t.Fatalf("lobby has no %q entry: %s", nickname, snapshot.Data)
	return false
}

// readCloseCode waits for the socket to end and returns the WebSocket close
// code the peer observed. A frame still in flight is ignored: the contract
// under test is how the connection ENDED.
func readCloseCode(t *testing.T, conn *websocket.Conn) int {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(testIOTimeout))
	for {
		_, _, err := conn.ReadMessage()
		if err == nil {
			continue
		}
		var closeErr *websocket.CloseError
		if errors.As(err, &closeErr) {
			return closeErr.Code
		}
		t.Fatalf("connection ended with %v, want a WebSocket close frame", err)
	}
}

// waitFor polls cond until it holds or the budget runs out. It exists for the
// one thing a transport test cannot observe synchronously: a SERVER-side
// teardown, which happens in the connection's own goroutine after the peer's
// socket has already ended.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}
