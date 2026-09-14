// Package hub owns the WebSocket surface. Phase 1 ships a temporary echo
// handler in this file so that Phase 2 can replace exactly this file with the
// real hub (engine-backed revisions, roles, resync).
package hub

import (
	"encoding/json"
	"log"
	"net/http"
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
	// writeWait bounds a single envelope write.
	writeWait = 5 * time.Second
)

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
// without a reply. One writer per connection, as gorilla requires.
func Echo(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response.
		log.Printf("ws: upgrade failed: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	conn.SetReadLimit(readLimit)

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
