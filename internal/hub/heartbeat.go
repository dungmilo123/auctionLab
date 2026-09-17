package hub

import (
	"sync"
	"time"
)

// This file owns D-132's application-level heartbeat: ONE hub-level goroutine
// that broadcasts a `heartbeat` control frame to every joined connection at
// Policy.HeartbeatPeriod, in every game phase.
//
// Why the server must send data and not just pings: a browser cannot observe a
// WebSocket protocol-level ping, and the quiet phases (LOBBY, ITEM_INTRO,
// SETTLED) produce no JSON frames at all — so on iOS a suspended page sees a
// socket whose readyState still claims "open" while nothing has arrived for
// minutes, and there is no way to notice. This beat is the only liveness
// evidence a page can timestamp: the client updates lastMessageAt on it and
// force-closes a socket that has been silent too long (research Pitfalls 5/6).
//
// The split matches ticker.go: StartHeartbeat is the production lifecycle
// (started and stopped by cmd/server), while heartbeat is one deterministic beat
// a test can call directly — no test sleeps on the 25-second production cadence.
//
// Like tunnel.status, the frame is control-plane: it carries the CURRENT
// revision without bumping it and is applied at any seq (protocol.go's frozen
// exception), so it can never be mistaken for a state update or a stream gap.

// heartbeatPayload is the beat's whole body: the server's millisecond clock and
// nothing else. Nothing role-specific, nothing stateful — a client stamps it and
// ignores the content.
type heartbeatPayload struct {
	TS int64 `json:"ts"`
}

// StartHeartbeat starts the single heartbeat goroutine and returns the stop
// function that ends it. The caller owns the lifetime: cmd/server starts it
// after the hub exists and defers the stop ahead of the ticker's, so no beat can
// outlive the store it reads (Pitfall 11). Stop is idempotent.
func (h *Hub) StartHeartbeat() (stop func()) {
	beats := time.NewTicker(h.policy.HeartbeatPeriod)
	done := make(chan struct{})
	var once sync.Once

	go func() {
		for {
			select {
			case <-done:
				return
			case <-beats.C:
				h.heartbeat()
			}
		}
	}()

	return func() {
		once.Do(func() {
			beats.Stop()
			close(done)
		})
	}
}

// heartbeat is one beat. It runs in EVERY phase — unlike tick, which describes a
// live window — because the connection that needs it is by definition the one
// the server believes is connected, and a quiet lobby is exactly when silence
// would otherwise be indistinguishable from a dead socket.
//
// It goes through engine.Section and the ordinary broadcast, so each frame takes
// a real seq on each recipient's lock-held FIFO at the current revision.
func (h *Hub) heartbeat() {}
