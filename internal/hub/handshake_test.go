package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// This file is the tracer's transport contract (D-23 / D-47): the three role
// handshakes, the pre-join guard, the unsupported-command answer and the
// malformed-frame / oversize paths — every one of them asserted on a real
// WebSocket, never on a mock (D-43).
//
// The assertions are written against JSON literals rather than the production
// catalog on purpose (D-24 freezes the wire, not the Go identifiers): renaming
// a constant must not be able to move this contract silently.

// tracerIOTimeout bounds every read and write so a handler that answers
// nothing fails the case instead of hanging the package.
const tracerIOTimeout = 5 * time.Second

// dialTracer starts an httptest server around the tracer handler and dials it
// with gorilla's DefaultDialer — the Phase 1 idiom (internal/hub/echo_test.go):
// the real transport, not a mocked connection.
//
// Note for the record (Pitfall 12 / T-04-03): the dialer sends no Origin
// header, so this idiom cannot exercise gorilla's same-origin CheckOrigin; that
// path is owned by TestOriginRejectionIsLoggedOncePerInterval.
func dialTracer(t *testing.T) *websocket.Conn {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(Echo))
	t.Cleanup(srv.Close)

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
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

// tracerFrame is one server frame decoded for a contract assertion: the five
// D-21 fields plus the raw key set of the object that actually arrived, so a
// case can assert the wire shape literally instead of trusting a Go struct.
type tracerFrame struct {
	Type string          `json:"type"`
	Seq  int64           `json:"seq"`
	Rev  int64           `json:"rev"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data"`

	keys []string
}

// tracerWrite sends one text frame and fails the case if the socket refuses it.
func tracerWrite(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()

	_ = conn.SetWriteDeadline(time.Now().Add(tracerIOTimeout))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatalf("writing %s: %v", payload, err)
	}
}

// tracerRead reads the next frame and fails the case if none arrives.
func tracerRead(t *testing.T, conn *websocket.Conn) tracerFrame {
	t.Helper()

	env, err := tracerReadErr(conn)
	if err != nil {
		t.Fatalf("reading a frame: %v", err)
	}
	return env
}

// tracerReadErr reads the next frame, or reports why none arrived. Separate
// from tracerRead so the "the server must stay silent here" paths can assert on
// the error instead of dying on it.
func tracerReadErr(conn *websocket.Conn) (tracerFrame, error) {
	_ = conn.SetReadDeadline(time.Now().Add(tracerIOTimeout))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		return tracerFrame{}, err
	}

	var env tracerFrame
	if err := json.Unmarshal(raw, &env); err != nil {
		return tracerFrame{}, fmt.Errorf("frame is not a JSON object: %v (%s)", err, raw)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return tracerFrame{}, fmt.Errorf("frame has no key set: %v (%s)", err, raw)
	}
	for key := range fields {
		env.keys = append(env.keys, key)
	}
	sort.Strings(env.keys)
	return env, nil
}

// tracerData decodes the frame's data object.
func tracerData(t *testing.T, env tracerFrame) map[string]json.RawMessage {
	t.Helper()

	var data map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("data is not a JSON object: %v (%s)", err, env.Data)
	}
	return data
}

// tracerString reads one string field out of a decoded data object.
func tracerString(t *testing.T, data map[string]json.RawMessage, key string) string {
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
func assertErrorCode(t *testing.T, env tracerFrame, want string) {
	t.Helper()

	if env.Type != "error" {
		t.Fatalf("frame type = %q, want \"error\" (with code %q)", env.Type, want)
	}
	if got := tracerString(t, tracerData(t, env), "code"); got != want {
		t.Fatalf("error code = %q, want %q", got, want)
	}
}

// TestHandshakePerRole is the tracer's reason for existing (RT-07 / D-23): each
// of the three roles reaches hello at seq 1 and then a role snapshot at seq 2,
// both carrying the same revision because no mutation happened in between.
func TestHandshakePerRole(t *testing.T) {
	for _, role := range []string{"player", "screen", "admin"} {
		t.Run(role, func(t *testing.T) {
			conn := dialTracer(t)
			tracerWrite(t, conn, fmt.Sprintf(`{"type":"join","role":%q}`, role))

			hello := tracerRead(t, conn)
			if hello.Type != "hello" {
				t.Fatalf("first frame type = %q, want \"hello\" (D-23: hello answers a join)", hello.Type)
			}
			if hello.Seq != 1 {
				t.Errorf("hello seq = %d, want 1 (D-21: seq starts at 1 on a fresh connection)", hello.Seq)
			}
			assertEnvelopeKeys(t, hello)
			if got := tracerString(t, tracerData(t, hello), "role"); got != role {
				t.Errorf("hello role = %q, want %q", got, role)
			}

			snapshot := tracerRead(t, conn)
			if snapshot.Type != "state.snapshot" {
				t.Fatalf("second frame type = %q, want \"state.snapshot\" (D-23: the snapshot follows hello)", snapshot.Type)
			}
			if snapshot.Seq != 2 {
				t.Errorf("snapshot seq = %d, want 2 (D-21: the snapshot is the second frame of the stream)", snapshot.Seq)
			}
			if snapshot.Rev != hello.Rev {
				t.Errorf("snapshot rev = %d, hello rev = %d, want one revision (D-23: no event can slip between the pair)",
					snapshot.Rev, hello.Rev)
			}
			assertEnvelopeKeys(t, snapshot)
			if got := tracerString(t, tracerData(t, snapshot), "phase"); got != "LOBBY" {
				t.Errorf("snapshot phase = %q, want \"LOBBY\" (PLAN.md §6: Phase 2 wires the machine only as far as LOBBY)", got)
			}
		})
	}
}

// TestPreJoinCommand pins D-23's guard: a command that is not a join is
// answered with not_joined, and the socket stays open for the client to join.
func TestPreJoinCommand(t *testing.T) {
	conn := dialTracer(t)
	tracerWrite(t, conn, `{"type":"bid.place","amount":1000}`)
	assertErrorCode(t, tracerRead(t, conn), "not_joined")

	// The same socket must still be usable: a rejected command is not a
	// disconnect (D-23 keeps the connection open for the corrected join).
	tracerWrite(t, conn, `{"type":"join","role":"player"}`)
	if got := tracerRead(t, conn).Type; got != "hello" {
		t.Fatalf("frame after the refused command = %q, want \"hello\" — the socket must stay open", got)
	}
}

// TestUnsupportedCommand pins D-24's forward-compatibility rule: a command from
// a later phase is already in the frozen catalog, so it answers the typed
// unsupported error instead of looking like an unknown message.
func TestUnsupportedCommand(t *testing.T) {
	conn := dialTracer(t)
	tracerWrite(t, conn, `{"type":"join","role":"player"}`)
	_ = tracerRead(t, conn) // hello
	_ = tracerRead(t, conn) // state.snapshot

	tracerWrite(t, conn, `{"type":"admin.item.start","id":"item-1"}`)
	assertErrorCode(t, tracerRead(t, conn), "unsupported")
}

// TestBadMessage pins the two malformed-input paths: a frame that is not JSON
// is answered with the typed bad_message error, and a frame over D-40's 4 KiB
// limit terminates the peer instead of buffering it (T-02-05).
func TestBadMessage(t *testing.T) {
	t.Run("a frame that is not JSON is answered with bad_message", func(t *testing.T) {
		conn := dialTracer(t)
		tracerWrite(t, conn, "this is not json")

		env, err := tracerReadErr(conn)
		if err != nil {
			t.Fatalf("no frame after a malformed message: %v (D-24 wants error{code:bad_message} on the same socket)", err)
		}
		assertErrorCode(t, env, "bad_message")
	})

	t.Run("a frame over the 4 KiB limit terminates the peer", func(t *testing.T) {
		conn := dialTracer(t)

		// Valid JSON, so the only reason it can be refused is the read limit.
		oversized := fmt.Sprintf(`{"type":"join","role":"player","pad":%q}`, strings.Repeat("a", 5<<10))

		_ = conn.SetWriteDeadline(time.Now().Add(tracerIOTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(oversized)); err != nil {
			// The server may already have closed while the frame was in flight;
			// that is termination too. The read below is the real assertion.
			t.Logf("oversized write errored (%v) — the server closed early", err)
		}

		// The contract is "the payload was not buffered and answered" — not a
		// specific close code: gorilla's on-limit behaviour (1009, ErrReadLimit)
		// is an implementation detail this test must not pin.
		if env, err := tracerReadErr(conn); err == nil {
			t.Fatalf("the server answered an oversized frame with %q; the D-40 read limit is not in effect", env.Type)
		}
	})
}
