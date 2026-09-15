package hub

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
)

// This file owns everything about ONE connection: the read pump that routes
// client frames, the single write pump that owns every application write, the
// keepalive, and the FIFO the engine's fan-out enqueues into.

// client is one live connection.
type client struct {
	hub  *Hub
	conn *websocket.Conn

	// Identity. It is written once, inside the engine's critical section by the
	// handshake (D-23), and read on the two paths that hold the same lock: the
	// fan-out and the resync snapshot. The read pump's own goroutine is the
	// writer, which is why no other goroutine may touch these fields.
	role     string
	playerID string

	// seq is this connection's stream counter (D-21). It is consumed at every
	// enqueue ATTEMPT — a drop leaves a visible gap, which is the signal the
	// client's gap rule consumes — and it is only ever mutated with the engine's
	// exclusive lock held, which is what makes the stream gapless by
	// construction rather than by luck.
	seq int64

	// rev is the last revision handed to this connection. The fan-out writes it
	// under the engine lock, the read pump reads it for a private error frame;
	// atomic so that read needs no lock.
	rev atomic.Int64

	// send is the per-connection FIFO. It is deliberately never closed: a
	// producer holding a stale registry snapshot is always one instruction away
	// from a send, and a close would panic inside the engine's critical section
	// (research Pitfall 6).
	send chan Envelope

	done      chan struct{}
	closeOnce sync.Once

	// retiring asks the writer to flush what is queued and then end the
	// connection. It is how the D-23 rejection cap closes a socket AFTER the
	// typed error that explains it, instead of racing it.
	retiring atomic.Bool

	// superseded records that a newer connection took this identity (D-31).
	superseded atomic.Bool

	// Counters for D-41's accounting.
	drops      atomic.Int64
	suppressed atomic.Int64
	resyncN    atomic.Int64
	queued     atomic.Int64

	// rejectedJoins counts D-23 rejections on this connection. It lives in the
	// read pump's goroutine and dies with the connection.
	rejectedJoins int
}

// inbound is the union of the client→server fields Phase 2 needs to route a
// frame. Unknown fields are ignored on purpose: D-24 freezes the catalog, not
// the payload structs, and later phases add optional fields to the same
// commands (D-24's additive rule).
type inbound struct {
	Type     string `json:"type"`
	Role     string `json:"role"`
	Nickname string `json:"nickname"` // consumed by the engine's identity resolution
	Token    string `json:"token"`    // consumed by the engine's identity resolution
	Key      string `json:"key"`      // ADMIN_KEY check is Phase 4 (D-24: accepted unchecked for now)
}

// newClient wires one upgraded connection to the hub.
func newClient(h *Hub, conn *websocket.Conn) *client {
	return &client{
		hub:  h,
		conn: conn,
		send: make(chan Envelope, h.policy.SendBuffer),
		done: make(chan struct{}),
	}
}

// armLiveness installs D-40's liveness policy on the connection: the frame cap,
// the read deadline and the pong handler that refreshes it. The ping ticker
// lives in writePump so one goroutine owns every write.
func (c *client) armLiveness() {
	c.conn.SetReadLimit(c.hub.policy.ReadLimit)
	_ = c.conn.SetReadDeadline(time.Now().Add(c.hub.policy.PongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.hub.policy.PongWait))
	})
}

// close ends the connection exactly once. It is safe to call from any
// goroutine and from the connection's own deferred teardown.
func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// retire asks for a graceful end: the writer drains what is already queued and
// then closes. The read pump stops reading, so nothing can be appended after
// the drain in practice, and the typed error that triggered the retirement is
// always written before the close frame.
func (c *client) retire() { c.retiring.Store(true) }

// readPump routes client frames until the socket ends. It is the only reader,
// and it is where the per-connection protocol state lives: nothing it does ever
// writes to the socket directly.
//
// A retired connection is NOT ended here: the writer owns the ending, so the
// typed error that triggered the retirement is always flushed before the close
// frame (D-23's cap must not look like a network drop).
func (c *client) readPump() {
	for {
		messageType, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			// The catalog is JSON text; anything else is malformed input and
			// gets the typed answer instead of silence (T-02-05).
			c.sendError(CodeBadMessage, "only text frames carry protocol messages")
			continue
		}
		c.handle(payload)
	}
}

// handle routes one text frame against the frozen catalog.
func (c *client) handle(payload []byte) {
	var in inbound
	if err := json.Unmarshal(payload, &in); err != nil {
		c.sendError(CodeBadMessage, "message is not valid JSON")
		return
	}
	if in.Type == "" {
		c.sendError(CodeBadMessage, "message has no type")
		return
	}

	// D-23: every command except join is refused before a successful join — and
	// the socket stays open, so the client can fix its nickname and retry
	// without a reload.
	if c.role == "" && in.Type != TypeJoin {
		c.sendError(CodeNotJoined, "join first")
		return
	}

	switch in.Type {
	case TypeJoin:
		c.handleJoin(in)
	case TypeResync:
		// D-42: the snapshot is built under the engine's exclusive lock and
		// enqueued into this connection's own FIFO, never written to the socket
		// from here.
		_ = c.hub.engine.Section(func(v game.View) error {
			c.resync(v)
			return nil
		})
	default:
		// D-24: the Phase 3–6 commands are already in the catalog, so a client
		// that sends one now gets a typed answer instead of silence.
		c.sendError(CodeUnsupported, "command not implemented in this phase")
	}
}

// handleJoin completes D-23's handshake for all three roles. The whole
// handshake — identity resolution, registry supersede, hello and the role
// snapshot — runs inside ONE engine.mu critical section, which is what makes
// "no event can slip between hello and the snapshot" a structural property
// rather than a hope.
func (c *client) handleJoin(in inbound) {
	if c.role != "" {
		c.sendError(CodeUnsupported, "this connection has already joined")
		return
	}
	switch in.Role {
	case RolePlayer, RoleScreen, RoleAdmin:
	default:
		c.sendError(CodeBadMessage, "join needs role player|screen|admin")
		return
	}

	_, err := c.hub.engine.Join(game.JoinRequest{
		Role:     game.Role(in.Role),
		Nickname: in.Nickname,
		Token:    in.Token,
		Key:      in.Key,
	}, c.handshake)
	if err == nil {
		return
	}

	// A rejection is a normal classroom event (two players typing the same
	// nickname), so the socket stays open for a corrected retry; the connection
	// is retired only once D-23's cap is reached (research Pitfall 13).
	var rejected *game.JoinError
	if errors.As(err, &rejected) {
		c.sendError(rejected.Code, rejected.Message)
		c.rejectedJoins++
		if c.rejectedJoins >= maxRejectedJoins {
			c.retire()
		}
		return
	}

	log.Printf("ws: join failed: %v", err)
	c.close()
}

// handshake is the emit callback engine.Join invokes with its exclusive lock
// held, after the identity is durable and before the fan-out of that same
// mutation. hello goes out first (seq 1) and the role projection second (seq 2),
// and no producer can enqueue between them.
func (c *client) handshake(hello game.Hello, snapshot any) {
	c.role = string(hello.Role)
	c.playerID = hello.PlayerID

	// D-31: a valid token means this connection now owns the identity, so any
	// earlier connection holding it is retired — with the dedicated close code,
	// before the new one is registered, so the old teardown cannot subsequently
	// run against a registration it no longer owns.
	if hello.PlayerID != "" {
		if old := c.hub.registry.lookup(hello.PlayerID); old != nil && old != c {
			c.hub.supersede(old)
		}
	}

	c.enqueue(hello.Rev, TypeHello, hello)
	c.enqueue(hello.Rev, TypeStateSnapshot, snapshot)
}

// resync enqueues one fresh role projection into this connection's own FIFO.
// It runs inside engine.Section, so the snapshot cannot be built at a revision
// a concurrent mutation has already overtaken (D-42).
func (c *client) resync(v game.View) {
	c.resyncN.Add(1)
	c.hub.resyncs.Add(1)
	c.enqueue(v.Rev(), TypeStateSnapshot, v.Snapshot(game.Role(c.role), c.playerID))
}

// enqueue hands one message to this connection's FIFO. It never blocks and
// never closes the channel.
//
// PRECONDITION: engine.mu is held exclusively (D-33). That is what makes
// c.seq++ and the send race-free: the registry read is lock-free, but no two
// enqueues for the same client can overlap only because every producer is
// inside the engine's critical section.
func (c *client) enqueue(rev int64, typ string, data any) {
	next := c.seq + 1

	payload, err := json.Marshal(data)
	if err != nil {
		// A projection that cannot be serialized is a projector bug, not a
		// client error. The seq is still consumed so the client sees the gap.
		c.seq = next
		log.Printf("ws: marshaling %s: %v", typ, err)
		return
	}

	c.seq = next
	c.rev.Store(rev)

	select {
	case c.send <- Envelope{Type: typ, Seq: next, Rev: rev, TS: c.hub.now().UnixMilli(), Data: payload}:
		c.queued.Add(1)
	default:
		// The FIFO is full: this client is not keeping up. Count it and drop
		// the message; 02-03 Task 2 owns the fence that repairs the stream.
		c.drops.Add(1)
		c.hub.drops.Add(1)
	}
}

// sendError writes the frozen typed error (D-24) at this connection's current
// revision: a private answer is not a state event and must never move rev.
func (c *client) sendError(code, message string) {
	c.enqueue(c.rev.Load(), TypeError, errorPayload{Code: code, Message: message})
}

// writePump is the ONLY writer of application envelopes on this connection. It
// owns the ping ticker too, so there is exactly one goroutine writing to the
// socket (WriteControl is the single exception gorilla allows, and it is used
// here and by the supersede path, which needs it to deliver close code 4001).
func (c *client) writePump() {
	ticker := time.NewTicker(c.hub.policy.PingPeriod)
	defer ticker.Stop()

	for {
		if c.retiring.Load() && len(c.send) == 0 {
			c.close()
			return
		}
		select {
		case <-c.done:
			return
		case env := <-c.send:
			c.queued.Add(-1)
			if !c.write(env) {
				return
			}
			if c.retiring.Load() && len(c.send) == 0 {
				c.close()
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(c.hub.policy.WriteWait)); err != nil {
				// Unreachable peer: close so the blocked read returns now
				// instead of at the deadline.
				c.close()
				return
			}
		}
	}
}

// write sends one envelope. It reports whether the connection survived.
func (c *client) write(env Envelope) bool {
	payload, err := json.Marshal(env)
	if err != nil {
		log.Printf("ws: marshaling the %s envelope: %v", env.Type, err)
		return true
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.hub.policy.WriteWait))
	if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		c.close()
		return false
	}
	return true
}
