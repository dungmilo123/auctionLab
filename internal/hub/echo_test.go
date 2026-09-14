package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialEcho starts an httptest server around Echo and dials it with gorilla's
// DefaultDialer — the real transport, not a mocked connection.
//
// Note for the record (Pitfall 12 / T-04-03): the dialer sends no Origin
// header, so this idiom cannot exercise gorilla's same-origin CheckOrigin. The
// browser path stays covered by the manual widget check in plan 01-05; this gap
// is documented, never silently assumed closed.
func dialEcho(t *testing.T) *websocket.Conn {
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

// TestEcho covers the Phase 1 WS surface end to end: the envelope round-trip,
// its exact field set (no revision field exists yet — D-01), the malformed
// frame path and the read limit.
func TestEcho(t *testing.T) {
	t.Run("round-trips the payload verbatim in a three-field envelope", func(t *testing.T) {
		conn := dialEcho(t)

		payload := map[string]any{
			"probe":  "echo-test",
			"n":      1,
			"nested": map[string]any{"ok": true},
		}
		sent, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshaling the probe: %v", err)
		}

		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, sent); err != nil {
			t.Fatalf("writing the probe: %v", err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("reading the echo: %v", err)
		}

		// 1. The decoded envelope's key set must be exactly {type, ts, data}.
		//    The exact-set assertion is the point: a future field addition has to
		//    be a deliberate, visible act, and a plausible-looking `seq` here would
		//    misrepresent the protocol as frozen before Phase 2 freezes it (D-01).
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("echo frame is not a JSON object: %v (%s)", err, raw)
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if want := []string{"data", "ts", "type"}; !reflect.DeepEqual(keys, want) {
			t.Fatalf("envelope keys = %v, want exactly %v (D-01: Phase 1 has no revision/sequence field)", keys, want)
		}

		// 2. Typed assertions on the three fields.
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decoding the envelope: %v", err)
		}
		if env.Type != "echo" {
			t.Errorf("type = %q, want \"echo\"", env.Type)
		}
		if env.TS <= 0 {
			t.Errorf("ts = %d, want a positive server millisecond clock", env.TS)
		}
		if string(env.Data) != string(sent) {
			t.Errorf("data = %s, want the payload verbatim %s", env.Data, sent)
		}

		// 3. Deep-equality of the decoded payload (types normalised by JSON).
		var gotData, wantData any
		if err := json.Unmarshal(env.Data, &gotData); err != nil {
			t.Fatalf("decoding data: %v", err)
		}
		if err := json.Unmarshal(sent, &wantData); err != nil {
			t.Fatalf("decoding the sent payload: %v", err)
		}
		if !reflect.DeepEqual(gotData, wantData) {
			t.Errorf("data = %#v, want %#v", gotData, wantData)
		}
	})

	t.Run("a non-JSON text frame is not echoed", func(t *testing.T) {
		conn := dialEcho(t)

		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, []byte("this is not json")); err != nil {
			t.Fatalf("writing the malformed frame: %v", err)
		}

		// A valid probe follows: the next frame back must be its echo, which
		// proves the malformed frame produced no reply at all (rather than
		// relying on a read timeout to conclude "nothing came back").
		probe := []byte(`{"probe":"after-malformed"}`)
		if err := conn.WriteMessage(websocket.TextMessage, probe); err != nil {
			t.Fatalf("writing the probe: %v", err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("reading the probe echo: %v", err)
		}
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decoding the probe echo: %v", err)
		}
		if string(env.Data) != string(probe) {
			t.Fatalf("reply = %s, want the probe's echo %s — a malformed frame must be dropped, not answered", raw, probe)
		}
	})

	t.Run("a frame over the 4 KiB read limit terminates the connection", func(t *testing.T) {
		conn := dialEcho(t)

		// Valid JSON, so the only reason it must not be echoed is the read limit.
		oversized, err := json.Marshal(map[string]string{"pad": strings.Repeat("a", readLimit)})
		if err != nil {
			t.Fatalf("building the oversized payload: %v", err)
		}
		if len(oversized) <= readLimit {
			t.Fatalf("test payload is %d bytes, not over the %d-byte limit", len(oversized), readLimit)
		}

		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, oversized); err != nil {
			// The server may already have closed while the frame was in flight;
			// that is termination too. The read below is the real assertion.
			t.Logf("oversized write errored (%v) — the server closed early", err)
		}

		// The assertion is "no echo, connection gone" — not a specific close code:
		// gorilla's on-limit behaviour (1009 close, ErrReadLimit) is an
		// implementation detail, while "the payload was not buffered and answered"
		// is the contract this test exists to pin.
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if _, raw, err := conn.ReadMessage(); err == nil {
			t.Fatalf("server returned a %d-byte message for an oversized frame; the read limit is not in effect", len(raw))
		} else {
			t.Logf("connection terminated after the oversized frame: %v", err)
		}
	})
}
