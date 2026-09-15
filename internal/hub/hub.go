package hub

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
)

// This file owns the hub: the registry of live connections, the join/registry
// lifecycle and the fan-out edge the engine calls with its exclusive lock held.
// The per-connection machinery (keepalive, the single writer, the drop/resync
// fence) lives in client.go.
//
// It replaces the Phase-2 echo skeleton wholesale (D-01/D-44): `Echo` and
// `serve` are gone, cmd/server registers `(*Hub).ServeWS` at /ws, and
// internal/hub/echo.go + echo_test.go were deleted in the same commit that
// deleted their last reference.

// maxRejectedJoins is D-23's DoS cap: a connection may be refused this many
// joins before the server closes it. It counts REJECTIONS, not attempts — a
// player who mistypes a nickname twice and then succeeds is served (research
// Pitfall 13), and the counter dies with the connection.
const maxRejectedJoins = 5

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

// Hub is the live-connection registry and the engine's fan-out edge. The engine
// hands it a committed revision and a lock-held view (D-33); the hub never takes
// a lock of its own on the enqueue path, and it never writes to a socket from
// the engine's goroutine.
type Hub struct {
	engine *game.Engine
	policy Policy
	now    func() time.Time

	registry registry

	// Counters are atomic and package-visible so the ordering tests can assert
	// the drop/resync accounting without a log scrape. D-41 defers any API built
	// on them.
	drops   atomic.Int64
	resyncs atomic.Int64
}

// New builds the hub for one engine. It is the only constructor cmd/server
// uses; newHub is the same thing with D-40's policy injected for tests.
func New(engine *game.Engine) *Hub {
	return newHub(engine, DefaultPolicy)
}

// newHub is New with the policy injected, so a test can pin the liveness
// mechanism and the FIFO depth with values short enough to observe (D-40).
func newHub(engine *game.Engine, policy Policy) *Hub {
	return &Hub{engine: engine, policy: policy, now: time.Now}
}

// registry is the copy-on-write set of live connections. Reads take no lock at
// all (one atomic pointer load); writes publish a fresh immutable slice. The
// mutex serializes writers only and is never taken while the engine's exclusive
// lock is held, so it cannot participate in a lock cycle (D-33).
type registry struct {
	ptr atomic.Pointer[[]*client]
	mu  sync.Mutex
}

// snapshot returns the current immutable slice. The caller must not mutate it.
func (r *registry) snapshot() []*client { return nil }

// add publishes a new registry that contains c.
func (r *registry) add(c *client) {}

// remove takes one client OUT of the registry. It is a compare-and-remove by
// POINTER IDENTITY, never by player id — an old connection's teardown must not
// be able to evict its successor (D-31).
func (r *registry) remove(c *client) bool { return false }

// lookup finds the live connection for one player id.
func (r *registry) lookup(playerID string) *client { return nil }

// ServeWS upgrades GET /ws and serves one connection for its lifetime.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response.
		logUpgradeFailure(err)
		return
	}
	_ = conn.Close()
}

// Enqueue is the engine's fan-out edge (D-33). The engine calls it with its
// exclusive lock held, after every committed mutation and every presence change.
func (h *Hub) Enqueue(rev int64, v game.View) {}

// Drops reports how many messages the hub dropped because a client's FIFO was
// full (D-41's drop/resync accounting).
func (h *Hub) Drops() int64 { return h.drops.Load() }

// Resyncs reports how many recovery snapshots the hub enqueued.
func (h *Hub) Resyncs() int64 { return h.resyncs.Load() }

// QueueDepth reports how many envelopes are buffered across every client. It is
// a gauge for logging and tests, never a control input.
func (h *Hub) QueueDepth() int64 { return 0 }
