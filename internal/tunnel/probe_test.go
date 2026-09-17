package tunnel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// This file owns the probe's failure CLASSIFICATION: the handshake the probe
// accepts (and the exact frame it sends), and the shape of every refusal it has
// to tell apart at the venue — a wrong role, a wrong sequence, a typed error, a
// refused upgrade, a silent server, a name that does not resolve.
//
// The distinction is the deliverable (D-126): a Go dial through a tunnel that
// dropped the `Upgrade` header fails as a 400, a dead tunnel fails as a
// timeout, and a tunnel being created fails as DNS. Those are three different
// incidents and the log has to say which one is happening.

// TestProbeHandshakeFrames pins what the probe SENDS and what it ACCEPTS: the
// join is the screen frame with no credential in it, and the answer must be the
// hello/snapshot pair at sequences 1 and 2.
func TestProbeHandshakeFrames(t *testing.T) {
	t.Run("sends exactly the screen join and accepts the frozen pair", func(t *testing.T) {
		joins := make(chan string, 1)
		url := scriptedServer(t, func(c *websocket.Conn) {
			_, raw, err := c.ReadMessage()
			if err != nil {
				return
			}
			joins <- string(raw)
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello","seq":1,"rev":0,"ts":1,"data":{"role":"screen"}}`))
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"state.snapshot","seq":2,"rev":0,"ts":1,"data":{}}`))
		})

		if err := probeOnce(url); err != nil {
			t.Fatalf("the frozen handshake was refused: %v", err)
		}
		select {
		case got := <-joins:
			if got != probeJoin {
				t.Fatalf("the probe sent %s, want exactly %s", got, probeJoin)
			}
		case <-time.After(time.Second):
			t.Fatal("the server never saw a join frame")
		}
	})

	t.Run("refuses a hello for another role", func(t *testing.T) {
		url := scriptedServer(t, func(c *websocket.Conn) {
			_, _, _ = c.ReadMessage()
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello","seq":1,"rev":0,"ts":1,"data":{"role":"player"}}`))
		})
		err := probeOnce(url)
		if err == nil {
			t.Fatal("a hello for the player role must fail a screen probe")
		}
		if !strings.Contains(err.Error(), "player") {
			t.Fatalf("the error does not name the role that arrived: %v", err)
		}
	})

	t.Run("refuses a hello that is not at seq 1", func(t *testing.T) {
		url := scriptedServer(t, func(c *websocket.Conn) {
			_, _, _ = c.ReadMessage()
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello","seq":3,"rev":0,"ts":1,"data":{"role":"screen"}}`))
		})
		err := probeOnce(url)
		if err == nil {
			t.Fatal("a hello at seq 3 must fail the probe")
		}
		if !strings.Contains(err.Error(), "seq 3") {
			t.Fatalf("the error does not report the sequence that arrived: %v", err)
		}
	})

	t.Run("names a typed refusal after the handshake started", func(t *testing.T) {
		url := scriptedServer(t, func(c *websocket.Conn) {
			_, _, _ = c.ReadMessage()
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello","seq":1,"rev":0,"ts":1,"data":{"role":"screen"}}`))
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","seq":2,"rev":0,"ts":1,"data":{"code":"rate_limited","message":"slow down"}}`))
		})
		err := probeOnce(url)
		if err == nil {
			t.Fatal("an error frame where the snapshot belongs must fail the probe")
		}
		if !strings.Contains(err.Error(), "rate_limited") {
			t.Fatalf("the error does not carry the refusal code: %v", err)
		}
	})

	t.Run("reports a 500 upgrade as a bad handshake", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		err := probeOnce(srv.URL)
		if err == nil {
			t.Fatal("an HTTP 500 on the upgrade must fail the probe")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Fatalf("the upgrade status must travel into the message: %v", err)
		}
	})

	t.Run("reports a name that does not resolve as a dial failure", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := ProbeURL(ctx, "https://no-such-host-a-bcdefg-hij.trycloudflare.com")
		if err == nil {
			t.Fatal("a name that does not resolve must fail the probe")
		}
		if !strings.Contains(err.Error(), "no-such-host-a-bcdefg-hij.trycloudflare.com") {
			t.Fatalf("the error does not name the host it dialed: %v", err)
		}
	})

	t.Run("refuses an origin that is not a tunnel", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		for _, origin := range []string{"", "ftp://a-b-c.trycloudflare.com", "not a url"} {
			err := ProbeURL(ctx, origin)
			if err == nil {
				t.Fatalf("ProbeURL(%q) must refuse an unusable origin", origin)
			}
			if !strings.Contains(err.Error(), "not a usable tunnel origin") {
				t.Fatalf("ProbeURL(%q) = %v, want the unusable-origin diagnostic", origin, err)
			}
		}
	})
}
