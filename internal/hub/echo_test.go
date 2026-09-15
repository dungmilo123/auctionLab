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

// TestEchoKeepalive pins the liveness mechanism (AGENTS.md §4 / WR-01) with
// timings small enough to observe: the server pings, a peer that answers the
// pings stays connected past the pong window, and a peer that stops answering
// is reclaimed instead of holding the handler goroutine forever.
func TestEchoKeepalive(t *testing.T) {
	// The mechanism is under test, not the Phase 2 numbers.
	ka := keepalive{pongWait: 500 * time.Millisecond, pingPeriod: 100 * time.Millisecond}

	dial := func(t *testing.T, tune func(*websocket.Conn)) *websocket.Conn {
		t.Helper()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			echo(w, r, ka)
		}))
		t.Cleanup(srv.Close)

		url := "ws" + strings.TrimPrefix(srv.URL, "http")
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("dialing %s: %v", url, err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		if tune != nil {
			tune(conn)
		}
		return conn
	}

	t.Run("pings keep a healthy silent peer connected past the pong window", func(t *testing.T) {
		conn := dial(t, nil)

		// The client must be reading for gorilla to service the server's pings
		// (control frames are handled on the read path) and answer them with a
		// pong — what a browser does natively.
		echoes := make(chan []byte, 4)
		go func() {
			for {
				_, raw, err := conn.ReadMessage()
				if err != nil {
					return
				}
				echoes <- raw
			}
		}()

		// Longer than the pong window and with no application traffic: without
		// the ping/pong refresh the read deadline would already have fired.
		time.Sleep(2 * ka.pongWait)

		probe := []byte(`{"probe":"still-alive"}`)
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.TextMessage, probe); err != nil {
			t.Fatalf("writing after %v of silence: %v", 2*ka.pongWait, err)
		}

		select {
		case raw := <-echoes:
			var env Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("decoding the echo: %v", err)
			}
			if string(env.Data) != string(probe) {
				t.Fatalf("data = %s, want the probe %s", env.Data, probe)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no echo after the pong window: the server dropped a live, ponging peer")
		}
	})

	t.Run("a peer that stops answering pings is reclaimed", func(t *testing.T) {
		conn := dial(t, func(c *websocket.Conn) {
			// Read (so the pings are processed) but never pong: a suspended
			// phone that still holds the socket open behaves exactly like this.
			c.SetPingHandler(func(string) error { return nil })
		})

		errc := make(chan error, 1)
		go func() {
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					errc <- err
					return
				}
			}
		}()

		select {
		case err := <-errc:
			t.Logf("server closed the silent peer after the pong window: %v", err)
		case <-time.After(10 * ka.pongWait):
			t.Fatal("a peer that never ponged is still connected; the read deadline is not enforced")
		}
	})
}
