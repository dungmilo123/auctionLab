package hub

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// This file owns everything about ONE connection: its read pump, its single
// write pump, the keepalive, and the FIFO the engine's fan-out enqueues into.

// client is one live connection.
type client struct {
	hub  *Hub
	conn *websocket.Conn

	// Identity, written once by the handshake inside the engine's critical
	// section (D-23) and read by the enqueue path, which holds the same lock.
	role     string
	playerID string

	// seq is this connection's stream counter (D-21). It is consumed at every
	// enqueue ATTEMPT and only ever mutated with the engine's exclusive lock
	// held, which is what makes a gapless stream a structural property.
	seq int64

	// send is the per-connection FIFO. It is deliberately never closed
	// (research Pitfall 6).
	send chan Envelope

	done      chan struct{}
	closeOnce sync.Once

	// Counters for D-41's accounting.
	drops      atomic.Int64
	suppressed atomic.Int64
	resyncN    atomic.Int64
	queued     atomic.Int64

	// rejectedJoins counts D-23 rejections on this connection. It lives in the
	// read pump's goroutine and dies with the connection.
	rejectedJoins int
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
// the read deadline and the pong handler that refreshes it.
func (c *client) armLiveness() {
	c.conn.SetReadLimit(c.hub.policy.ReadLimit)
	_ = c.conn.SetReadDeadline(time.Now().Add(c.hub.policy.PongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(c.hub.policy.PongWait))
	})
}

// close ends the connection exactly once.
func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}
