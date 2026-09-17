package hub

import (
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/api"
	"auctionlab/internal/game"
)

// This file owns the hub: the registry of live connections, the join/registry
// lifecycle and the fan-out edge the engine calls with its exclusive lock held.
// The per-connection machinery (keepalive, the single writer, the FIFO) lives
// in client.go.
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

// closeCodeSuperseded is D-31's dedicated close code: the peer that lost its
// identity to a newer connection is told so explicitly, because "someone signed
// in on another device" is a different situation from a network drop and the
// client is expected to say so rather than reconnect in a loop.
const closeCodeSuperseded = 4001

// supersedeWriteWait bounds the close frame the supersede path sends. It is
// deliberately far below the policy's WriteWait because that write runs inside
// the engine's exclusive critical section: WriteControl flushes to the peer, so
// a phone that has stopped reading (the case the FIFO fence exists for) would
// otherwise hold engine.mu — and with it every mutation and every other
// connection's error reply — for the full deadline. A responsive peer still
// observes close code 4001; a wedged one just sees the socket end.
const supersedeWriteWait = 250 * time.Millisecond

// closeCodeSessionEnded is D-78/D-80's dedicated close code: the PRESENTER ended
// this session — a cleared room, or one identity released — so the device is
// expected to return to the join form and may claim its nickname again.
//
// It is deliberately not closeCodeSuperseded (4001): a phone that reads "someone
// signed in on another device" would show the wrong story, and the cleared or
// released player is not being replaced by anybody (A7).
const closeCodeSessionEnded = 4002

// endedWriteWait bounds the close frame the deliberate-cleanup paths send. Like
// supersedeWriteWait it is short, for the same reason: this write is synchronous
// to the peer, and a wedged phone must not hold the presenter's command for the
// policy's full WriteWait. Unlike the supersede path these calls run after the
// engine lock has been released, so a slow peer delays only its own connection.
const endedWriteWait = 250 * time.Millisecond

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

	// adminKey is the resolved ADMIN_KEY (D-74), injected at boot by cmd/server —
	// the one component that knows the env/persisted/minted precedence. It is
	// only ever COMPARED (authorizedAdmin), never logged and never echoed, and it
	// deliberately does not travel into game.Config: the admin projection is
	// built from state, and a capability that reached state would be one field
	// away from a browser (D-74/T-04-04).
	adminKey string

	registry registry

	// Counters are atomic and package-visible so the ordering tests can assert
	// the drop/resync accounting without a log scrape. D-41 defers any API built
	// on them.
	drops   atomic.Int64
	resyncs atomic.Int64
}

// New builds the hub for one engine and one resolved admin capability. It is the
// only constructor cmd/server uses; newHub is the same thing with D-40's policy
// injected for tests.
func New(engine *game.Engine, adminKey string) *Hub {
	return newHub(engine, DefaultPolicy, adminKey)
}

// NewWithPolicy is New with the D-40/D-132 policy injected.
//
// It exists for one reason: cmd/server's integration test drives the PRODUCTION
// heartbeat lifecycle (startLiveness) and needs a beat cadence short enough to
// observe, exactly as this package's own tests use the unexported newHub. The
// production constructor stays New, so a caller that does not think about the
// policy always gets DefaultPolicy's numbers.
func NewWithPolicy(engine *game.Engine, policy Policy, adminKey string) *Hub {
	return newHub(engine, policy, adminKey)
}

// newHub is New with the policy injected, so a test can pin the liveness
// mechanism and the FIFO depth with values short enough to observe (D-40).
func newHub(engine *game.Engine, policy Policy, adminKey string) *Hub {
	return &Hub{engine: engine, policy: policy, now: time.Now, adminKey: adminKey}
}

// registry is the copy-on-write set of live connections. Reads take no lock at
// all (one atomic pointer load); writers publish a fresh immutable slice. The
// mutex serializes writers only and is never taken while the engine's exclusive
// lock is held, so it cannot participate in a lock cycle (D-33).
type registry struct {
	ptr atomic.Pointer[[]*client]
	mu  sync.Mutex
}

// snapshot returns the current immutable slice. The caller must not mutate it.
func (r *registry) snapshot() []*client {
	cur := r.ptr.Load()
	if cur == nil {
		return nil
	}
	return *cur
}

// add publishes a new registry that contains c. Adding the same pointer twice
// is a no-op, so a caller that registers on upgrade and again on join cannot
// duplicate an entry.
func (r *registry) add(c *client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cur := r.snapshot()
	for _, existing := range cur {
		if existing == c {
			return
		}
	}
	next := make([]*client, 0, len(cur)+1)
	next = append(next, cur...)
	next = append(next, c)
	r.ptr.Store(&next)
}

// remove takes one client OUT of the registry. It is a compare-and-remove by
// POINTER IDENTITY, never by player id — an old connection's teardown must not
// be able to evict its successor (D-31). It reports whether this call was the
// one that removed c.
func (r *registry) remove(c *client) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	cur := r.snapshot()
	next := make([]*client, 0, len(cur))
	removed := false
	for _, existing := range cur {
		if existing == c {
			removed = true
			continue
		}
		next = append(next, existing)
	}
	if !removed {
		return false
	}
	r.ptr.Store(&next)
	return true
}

// lookup returns the FIRST registered connection for one player id, in
// registration order, or nil. Unjoined connections have no identity and are
// never returned.
//
// First match, not "the live one" (WR-05): a supersede leaves the old connection
// REGISTERED until its own teardown runs, so during that window two entries can
// share an id and the older one may already be closed. The handshake only needs
// the entry it is replacing, which is exactly the first match; a caller that
// must reach every holder scans the snapshot instead (endPlayerSession).
func (r *registry) lookup(playerID string) *client {
	if playerID == "" {
		return nil
	}
	for _, c := range r.snapshot() {
		if c.playerID() == playerID {
			return c
		}
	}
	return nil
}

// ServeWS upgrades GET /ws and serves one connection for its lifetime: one
// writer goroutine drains the FIFO, this goroutine reads, and the deferred
// teardown runs exactly once however the socket ends.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response.
		logUpgradeFailure(err)
		return
	}

	c := newClient(h, conn)
	// D-76: this is the only place the *http.Request exists — the admin role
	// arrives in the first frame, long after the request is gone — so the
	// capability cookie is captured here and the decision happens in handleJoin.
	// The value is never logged (T-04-04).
	c.adminCookie = requestedAdminCookie(r)
	c.armLiveness()
	h.registry.add(c)
	go c.writePump()

	defer func() {
		c.close()     // ends the writer; the send channel is never closed (Pitfall 6)
		h.teardown(c) // registry + presence, guarded by pointer identity (D-31)
	}()

	c.readPump()
}

// requestedAdminCookie reads D-75's capability cookie off the upgrade request.
//
// r.Cookie matches ONE named cookie, so nothing else the browser sends can
// influence the decision, and an absent cookie is reported as "" — which
// authorizedAdmin can never accept (api.Equal refuses an empty value outright).
func requestedAdminCookie(r *http.Request) string {
	cookie, err := r.Cookie(api.AdminCookieName)
	if err != nil {
		return "" // no cookie: handleJoin still has D-76's join.key channel
	}
	return cookie.Value
}

// authorizedAdmin answers D-76's admin-join decision: the capability arrives
// either as the cookie the browser sent with the upgrade request or as the `key`
// field a script/probe carries in the join frame, and BOTH are compared through
// the phase's one fixed-width comparison (api.Equal, T-04-01) — a second
// implementation of the comparison is exactly what reusing it prevents.
//
// Fail-closed by construction: api.Equal hashes both sides to a fixed width and
// refuses an empty supplied value, so a hub built without a key — or a join
// offering only an empty cookie — authorizes nobody (T-04-03). Neither value is
// ever logged or echoed.
func (h *Hub) authorizedAdmin(cookie, key string) bool {
	return api.Equal(cookie, h.adminKey) || api.Equal(key, h.adminKey)
}

// Enqueue fans one committed revision out as each connection's own role
// projection. It is the D-33 fan-out edge for every mutation whose frame IS a
// snapshot — a join, a presence change, resync and drop recovery — and the
// fallback Fanout uses when a mutation recorded no typed event of its own (H2).
//
// PRECONDITION: engine.mu is held exclusively. The registry is lock-free to
// read, but the per-client seq counter and the send channel are only race-free
// because no two enqueues for the same client can overlap — and only the engine
// lock guarantees that. Every enqueue is non-blocking: a client that cannot keep
// up loses its message (and is fenced for repair), never the room's.
func (h *Hub) Enqueue(rev int64, v game.View) {
	for _, c := range h.registry.snapshot() {
		if c.role() == "" {
			continue // no handshake yet: this connection has no role projection
		}
		c.enqueue(rev, TypeStateSnapshot, c.snapshot(v))
	}
}

// Fanout is the engine's fan-out edge (D-33/D-57). The engine calls it with its
// exclusive lock held, handing over the committed revision and the exact ordered
// event list the mutation recorded.
//
// PRECONDITION: engine.mu is held exclusively — the same rule as Enqueue. Every
// frame goes through the connection's own FIFO, so D-41's drop/fence/resync
// semantics apply to event frames exactly as they apply to a snapshot.
//
// Three rules make up the contract:
//
//   - A private event (To != "") reaches its addressee alone (D-53's
//     `bid.rejected` shape); a public one is broadcast to every joined role.
//   - The zero event — game.EventSnapshot — is per client by definition: it is
//     that connection's own role projection, built from the same lock-held view.
//   - An EMPTY list means the mutation produced no typed frame of its own (a
//     join, a presence change): the snapshot is the frame, so the established
//     D-33 path is reused unchanged.
//
// When the list moved numbers a client is watching — entering LIVE
// (`item.live`) or an accepted bid, an anti-snipe extension included
// (`bid.accepted`) — one D-56 `price.tick` follows the events, built from this
// same lock-held view. Nobody waits up to a tick period to see a deadline move.
func (h *Hub) Fanout(rev int64, v game.View, events []game.Event) {
	if len(events) == 0 {
		h.Enqueue(rev, v)
		return
	}

	// One tick per mutation, not one per client: the payload derives from the
	// lock-held state alone, and every client is owed the same numbers.
	var (
		tick    game.PriceTick
		hasTick bool
	)
	if wantsImmediateTick(events) {
		tick, hasTick = v.Tick()
	}

	for _, c := range h.registry.snapshot() {
		if c.role() == "" {
			continue
		}
		for _, event := range events {
			switch {
			case event.Type == game.EventSnapshot:
				c.enqueue(rev, TypeStateSnapshot, c.snapshot(v))
			case event.To != "" && event.To != c.playerID():
				// Addressed to one connection, and this is not it (D-53).
			default:
				c.enqueue(rev, event.Type, event.Data)
			}
		}
		if hasTick {
			c.enqueue(rev, TypePriceTick, tick)
		}
	}
}

// wantsImmediateTick reports whether a mutation's event list moved the numbers a
// D-56 tick carries: entering LIVE, or an accepted bid (including the anti-snipe
// extension a bid may have performed).
func wantsImmediateTick(events []game.Event) bool {
	for _, event := range events {
		if event.Type == game.EventItemLive || event.Type == game.EventBidAccepted {
			return true
		}
	}
	return false
}

// broadcast sends one already-built payload to every joined connection. It is
// the sibling-message primitive: a single revision may legitimately produce
// several messages (D-21), each taking its own seq.
//
// PRECONDITION: engine.mu is held exclusively — same rule as Enqueue.
func (h *Hub) broadcast(rev int64, typ string, data any) {
	h.broadcastTo(rev, typ, data)
}

// broadcastTo is broadcast with an AUDIENCE: no roles means every joined
// connection (the ticker's shape), and a non-empty role list restricts the
// fan-out to those roles — which is how a control frame reaches the two roles
// that render it and never a player (D-123/T-07-05). A connection that has not
// joined has no role and is never a recipient, exactly as in broadcast.
//
// PRECONDITION: engine.mu is held exclusively — same rule as Enqueue.
func (h *Hub) broadcastTo(rev int64, typ string, data any, roles ...string) {
	for _, c := range h.registry.snapshot() {
		role := c.role()
		if role == "" {
			continue
		}
		if len(roles) > 0 && !slices.Contains(roles, role) {
			continue
		}
		c.enqueue(rev, typ, data)
	}
}

// tunnelStatusPayload is the tunnel.status control-plane body (D-123): the
// origin the supervisor published, and whether a real screen-role WebSocket
// round-trip through it has succeeded. The zero value is a declared state — "no
// tunnel rung" — not an absence.
type tunnelStatusPayload struct {
	URL     string `json:"url"`
	Healthy bool   `json:"healthy"`
}

// BroadcastTunnelStatus publishes one tunnel status change to the roles that act
// on it — admin and screen — and never to a player (D-123/T-07-05). It is the
// hub half of cmd/server's status callback; the callback also hands the same
// status to the engine's runtime URL field, which is what moves the QR (D-128).
//
// The frame is control-plane by the catalog's own exception: it carries the
// CURRENT revision without bumping it and the client applies it regardless of
// seq. It is a fan-out like any other, so it takes a real seq per recipient on
// the lock-held FIFO.
//
// PRECONDITION: engine.mu is NOT held. The enqueue runs inside engine.Section,
// exactly like sendError and confirmNoop, because it writes each recipient's seq
// counter and FIFO (D-21/D-33).
func (h *Hub) BroadcastTunnelStatus(url string, healthy bool) {
	_ = h.engine.Section(func(v game.View) error {
		h.broadcastTo(v.Rev(), TypeTunnelStatus, tunnelStatusPayload{URL: url, Healthy: healthy}, RoleAdmin, RoleScreen)
		return nil
	})
}

// sendError answers ONE connection with the frozen typed error (D-24). It is
// the hub's private-error producer, and it takes the engine's exclusive lock for
// exactly the reason the fan-out runs under it: it writes that connection's seq
// counter and FIFO, and every producer that touches them must be inside
// engine.mu (D-21/D-33 — the registry read is lock-free, so the lock is the only
// thing that makes `c.seq++` race-free). A read pump that enqueued on its own
// could hand the same seq to two different messages, and the browser gate would
// silently discard the second as a duplicate (GAP 1).
//
// A private answer is not a state event: it carries the CURRENT revision and
// never bumps it (D-35). Like every other enqueue it is non-blocking, so a
// client that cannot keep up loses the frame and is fenced for repair rather
// than stalling the room (D-41/T-02-10).
func (h *Hub) sendError(c *client, code, message string) {
	_ = h.engine.Section(func(v game.View) error {
		c.enqueue(v.Rev(), TypeError, errorPayload{Code: code, Message: message})
		return nil
	})
}

// confirmNoop answers ONE connection whose command produced NO frame of its own
// — a keep_players reset of an already-fresh lobby (D-78), a release of a row
// that was already released (D-80) — with its own role snapshot.
//
// The admin panel derives every in-flight flag from its snapshot generation
// ("the snapshot is the confirmation"), so a command that fans out nothing
// would leave the controls it gated disabled until some unrelated frame
// arrived. This is the private counterpart of that rule: the connection that
// issued the command, its own projection, and the CURRENT revision, because a
// no-op is not a state event (D-35) — the same shape as sendError.
//
// PRECONDITION: engine.mu is NOT held. The caller runs after the command has
// returned; the snapshot is built inside engine.Section like every other
// producer that writes this connection's seq counter and FIFO (D-21/D-33).
func (h *Hub) confirmNoop(c *client) {
	_ = h.engine.Section(func(v game.View) error {
		c.enqueue(v.Rev(), TypeStateSnapshot, c.snapshot(v))
		return nil
	})
}

// recover is the consumer half of D-41's fence, run by the connection's writer
// when a producer has dropped a message.
//
// The order is the whole contract: discard the stale queue, then build the
// projection and enqueue it under the engine's EXCLUSIVE lock, then clear the
// fence under that same lock. Clearing it there is what guarantees the snapshot
// is the first thing the client sees after the gap — no producer can slip a
// message in between. Nothing here writes to the socket: the writer drains the
// FIFO afterwards, so the snapshot cannot overtake queued messages either.
func (h *Hub) recover(c *client) {
	c.drain()

	_ = h.engine.Section(func(v game.View) error {
		if c.enqueueLocked(v.Rev(), TypeStateSnapshot, c.snapshot(v), true) {
			c.fence.Store(false) // no producer can interleave: the lock is held
			// A dropped snapshot is not a repair: the fence stays armed and the
			// writer retries, so only the enqueue that actually succeeded may
			// count as a resync (D-41's accounting is what a health panel reads).
			c.resyncN.Add(1)
			h.resyncs.Add(1)
		}
		return nil
	})
}

// applyResync is one writer pass over a pending resync request (D-42). It goes
// through the same FIFO as every other message, so the snapshot cannot jump
// ahead of — or fall behind — the events already queued for this connection.
func (h *Hub) applyResync(c *client) {
	c.wantsResync.Store(false)

	_ = h.engine.Section(func(v game.View) error {
		if c.enqueueLocked(v.Rev(), TypeStateSnapshot, c.snapshot(v), false) {
			// A dropped snapshot is not a repair (recover's rule): only the
			// enqueue that delivered one may count. This drop has already armed
			// the fence, so the writer repairs through recover — which counts
			// the repair once, on success.
			c.resyncN.Add(1)
			h.resyncs.Add(1)
		}
		return nil
	})
}

// supersede retires the connection that used to hold an identity (D-31). It is
// called with the engine's lock held, from the successor's handshake section, so
// the replacement cannot race the registration it is replacing.
//
// WriteControl is the one write gorilla documents as safe to call concurrently
// with the connection's writer goroutine — and it is synchronously flushed, so
// the close frame reaches the peer before the socket ends. Its deadline is
// supersedeWriteWait, not the policy's WriteWait, because this runs with the
// engine's lock held and a wedged peer must not stall the room.
func (h *Hub) supersede(old *client) {
	old.superseded.Store(true)
	_ = old.conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(closeCodeSuperseded, "this identity joined on another connection"),
		time.Now().Add(supersedeWriteWait))
	old.close()
}

// endSession retires ONE connection because the presenter ended its session
// (D-78's clear_players, D-80's release). WriteControl is the one write gorilla
// documents as safe beside the connection's own writer goroutine, and it is
// flushed synchronously, so the peer observes close code 4002 rather than a
// silent drop. A socket that is already gone simply answers an error, which the
// idempotent close below makes harmless.
//
// PRECONDITION: engine.mu is NOT held. Every caller runs after the mutation it
// follows has returned — the opposite of the fan-out's contract — because ending
// a session is a deliberate per-socket act, not a state change to broadcast.
func (h *Hub) endSession(c *client) {
	_ = c.conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(closeCodeSessionEnded, "this session was ended by the presenter; join again"),
		time.Now().Add(endedWriteWait))
	c.close()
}

// endPlayerSession ends EVERY connection holding playerID. It is the single
// "this identity's device must notice" primitive: D-80's release, and D-78's
// clear_players once per id the commit deleted. An identity with no live socket
// is a no-op — the durable half of a release is the token write, which the
// engine has already committed, and a cleared row's phone may well have dropped
// off the network before the presenter clicked.
//
// Every holder, not the first registry match (WR-05). A supersede closes the old
// connection's socket but leaves it REGISTERED until its own teardown runs
// (D-31), so for that window the registry holds two connections under one id —
// and lookup's first match is the OLDEST, i.e. the stale one. A lookup-based
// primitive would end the dead socket and leave the live successor running: the
// presenter's clear or release would never reach the device it names, and the
// successor would keep rendering an identity the commit deleted. endSession is
// idempotent (closeOnce, and WriteControl answers an error on a dead socket), so
// ending an already-ended holder costs nothing.
//
// An empty id ends nothing: it is what every unjoined, screen and admin
// connection reports, so the guard is what keeps this scan from closing the
// room. It is the same rule lookup enforces by refusing "".
//
// PRECONDITION: engine.mu is NOT held — the same rule as endSession. The
// registry is read lock-free, and a phone that joins while this loop runs is a
// brand-new row the mutation never touched (WR-03).
func (h *Hub) endPlayerSession(playerID string) {
	if playerID == "" {
		return
	}
	for _, c := range h.registry.snapshot() {
		if c.playerID() == playerID {
			h.endSession(c)
		}
	}
}

// teardown runs once per connection, after its socket has ended.
//
// The compare-and-remove is by pointer identity: if a successor has already
// taken this player's registration, remove reports false and presence is left
// alone. That is the whole point of D-31 — the failure it prevents is a phone
// that reconnected being disconnected by its own previous connection's dying
// breath, which looks to the player like a live page that receives nothing.
func (h *Hub) teardown(c *client) {
	if !h.registry.remove(c) {
		return
	}
	if c.playerID() == "" {
		return // never joined: no presence to flip
	}
	if c.superseded.Load() {
		return // the identity moved on; its new connection owns presence now
	}
	if h.registry.lookup(c.playerID()) != nil {
		return // belt and braces: somebody else is registered for this identity
	}
	_ = h.engine.Disconnect(c.playerID())
}

// Drops reports how many messages the hub dropped because a client's FIFO was
// full (D-41's drop/resync accounting).
func (h *Hub) Drops() int64 { return h.drops.Load() }

// Resyncs reports how many recovery snapshots the hub enqueued.
func (h *Hub) Resyncs() int64 { return h.resyncs.Load() }

// QueueDepth reports how many envelopes are buffered across every client. It is
// a gauge for logging and tests, never a control input.
func (h *Hub) QueueDepth() int64 {
	var depth int64
	for _, c := range h.registry.snapshot() {
		depth += c.queued.Load()
	}
	return depth
}
