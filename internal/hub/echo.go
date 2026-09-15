// Package hub owns the WebSocket surface. Phase 1 ships a temporary echo
// handler in this file so that Phase 2 can replace exactly this file with the
// real hub (engine-backed revisions, roles, resync).
package hub

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Envelope is the wire shape of a Phase 1 echo reply. It carries exactly three
// fields: type, ts, data.
//
// There is deliberately NO seq/revision field: revision is Phase 2 semantics,
// and emitting a plausible-looking sequence number here would misrepresent the
// protocol as frozen before it is.
type Envelope struct {
	Type string          `json:"type"` // "echo"
	TS   int64           `json:"ts"`   // server clock, milliseconds
	Data json.RawMessage `json:"data"` // the received payload, verbatim
}

const (
	// readLimit caps a single client frame at 4 KiB (V5 input validation).
	readLimit = 4 << 10
	// writeWait bounds a single envelope or control-frame write.
	writeWait = 5 * time.Second
)

// keepalive holds one connection's liveness timings. The mechanism (read
// deadline + pong refresh + ping ticker) belongs here, with the socket that
// reads; ROADMAP defers the final numbers to Phase 2, which is why the values
// are parameters rather than constants.
type keepalive struct {
	pongWait   time.Duration // silence longer than this means the peer is gone
	pingPeriod time.Duration // ping cadence; must be shorter than pongWait
}

// defaultKeepalive follows AGENTS.md §4: ping at 90% of the pong window, so a
// live peer always refreshes the deadline before it expires.
var defaultKeepalive = keepalive{
	pongWait:   60 * time.Second,
	pingPeriod: 54 * time.Second, // 90% of pongWait
}

// originRejectLogInterval bounds how often the routine cross-origin rejection
// is logged: /ws is unauthenticated, so once the app is published any scanner
// can otherwise drive unbounded log writes at one line per 403.
const originRejectLogInterval = time.Minute

// lastOriginRejectLog is the unix-nanosecond time of the last logged origin
// rejection; 0 means none has been logged yet.
var lastOriginRejectLog atomic.Int64

// logUpgradeFailure logs a failed upgrade, collapsing the routine cross-origin
// rejection to at most one line per interval. Every other upgrade failure (a
// plain GET reaching /ws, a malformed handshake) is still logged every time —
// those are unexpected and worth the line.
func logUpgradeFailure(err error) {
	// gorilla answers a failed same-origin check with a HandshakeError whose
	// message names CheckOrigin; the wording is pinned by go.mod's v1.5.3. If it
	// ever changes, the fallback is one extra line per minute — never a missed
	// error.
	var handshake websocket.HandshakeError
	if errors.As(err, &handshake) && strings.Contains(err.Error(), "origin not allowed") {
		now := time.Now().UnixNano()
		if last := lastOriginRejectLog.Load(); last != 0 && now-last < int64(originRejectLogInterval) {
			return
		}
		lastOriginRejectLog.Store(now)
	}
	log.Printf("ws: upgrade failed: %v", err)
}

// upgrader leaves CheckOrigin nil on purpose: gorilla's default requires the
// Origin host to equal the Host header, and that same-origin check is this
// project's cross-site WebSocket hijacking defence. Never blanket-allow it for
// dev convenience.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Echo upgrades GET /ws and answers exactly one echo envelope per received
// text frame. Non-text frames and payloads that are not valid JSON are dropped
// without a reply. One writer per connection, as gorilla requires. Connections
// are pinged, and a peer that stops answering pongs is disconnected
// (AGENTS.md §4) — a vanished phone must not hold a goroutine forever.
func Echo(w http.ResponseWriter, r *http.Request) {
	echo(w, r, defaultKeepalive)
}

// echo is Echo with the liveness timings injected, so the keepalive behaviour
// can be pinned by tests without waiting out a real pong window.
func echo(w http.ResponseWriter, r *http.Request, ka keepalive) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response.
		logUpgradeFailure(err)
		return
	}
	defer func() { _ = conn.Close() }()

	// Liveness (AGENTS.md §4 / threat V5): a peer that vanishes without a close
	// frame — phone suspended, Wi-Fi drop, NAT rebind — would otherwise hold
	// this goroutine and its file descriptor until the kernel's TCP keep-alive
	// gives up. The read deadline is the enforcement; the pong handler is what
	// spares a healthy but silent peer from it.
	conn.SetReadLimit(readLimit)
	_ = conn.SetReadDeadline(time.Now().Add(ka.pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(ka.pongWait))
	})

	// Ping from a single ticker goroutine: WriteControl is the one write
	// gorilla allows concurrently with the envelope writer in the loop below.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(ka.pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
					// Unreachable peer: close so the blocked read returns now
					// instead of at the deadline.
					_ = conn.Close()
					return
				}
			}
		}
	}()

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			continue
		}
		if !json.Valid(payload) {
			continue
		}

		envelope := Envelope{
			Type: "echo",
			TS:   time.Now().UnixMilli(),
			Data: json.RawMessage(payload),
		}

		_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err := conn.WriteJSON(envelope); err != nil {
			return
		}
	}
}
