package hub

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
)

// This file owns everything about ONE connection: the read pump that routes
// client frames, the single write pump that owns every application write, the
// keepalive, and the FIFO the engine's fan-out enqueues into.

// identity is a connection's resolved role and player id. It is published once,
// inside the engine's critical section, and read from paths that hold no lock at
// all (the registry lookup, a teardown, a test's white-box probe), so it lives
// behind an atomic pointer and is never mutated afterwards.
type identity struct {
	role     string
	playerID string
}

// client is one live connection.
type client struct {
	hub  *Hub
	conn *websocket.Conn

	// adminCookie is D-75's capability cookie exactly as it arrived on the HTTP
	// upgrade request (D-76). ServeWS writes it once, in the same goroutine that
	// later reads it inside handleJoin, so it needs no atomic — and it is never
	// logged, never echoed and never sent to the engine (T-04-04).
	adminCookie string

	// who is the resolved identity (D-23). It is written once by the handshake
	// and read by the fan-out, the resync paths and the registry.
	who atomic.Pointer[identity]

	// seq is this connection's stream counter (D-21). It is consumed at every
	// enqueue ATTEMPT — a drop leaves a visible gap, which is the signal the
	// client's gap rule consumes — and it is only ever mutated with the engine's
	// exclusive lock held, which is what makes the stream gapless by
	// construction rather than by luck.
	seq int64

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

	// fence is D-41's producer latch: a dropped message arms it, every later
	// producer suppresses instead of delivering half a burst, and only the
	// writer's recovery pass clears it (under the engine lock).
	fence atomic.Bool

	// wantsResync is D-42's client request, kept as a flag so that several rapid
	// `resync` commands coalesce into one snapshot.
	wantsResync atomic.Bool

	// Counters for D-41's accounting.
	drops      atomic.Int64
	suppressed atomic.Int64
	resyncN    atomic.Int64
	queued     atomic.Int64

	// rejectedJoins counts D-23 rejections on this connection. It lives in the
	// read pump's goroutine and dies with the connection.
	rejectedJoins int
}

// inbound is the union of the client→server fields the hub needs to route a
// frame. Unknown fields are ignored on purpose: D-24 freezes the catalog, not
// the payload structs, and later phases add optional fields to the same
// commands (D-24's additive rule).
type inbound struct {
	Type     string `json:"type"`
	Role     string `json:"role"`
	Nickname string `json:"nickname"` // consumed by the engine's identity resolution
	Token    string `json:"token"`    // consumed by the engine's identity resolution
	Key      string `json:"key"`      // ADMIN_KEY check is Phase 4 (D-24: accepted unchecked for now)

	// Phase 3 (D-52/D-66), additive:
	//
	//   amount — `bid.place`'s amount, kept as the RAW JSON literal so that the
	//            D-52 ladder, not the decoder, owns every refusal (WR-01). The
	//            hub reads it with amountFromLiteral; a literal that is not a
	//            whole number reaches the ladder as 0, i.e. `invalid_amount`.
	//   id     — the item an `admin.item.*` command names.
	//   item   — D-66's full-object upsert payload; nil means the frame carried
	//            no `item` key at all, which is a bad_message rather than an
	//            empty create.
	//   ids    — the complete permutation `admin.item.reorder` demands.
	Amount json.RawMessage `json:"amount"`
	ID     string          `json:"id"`
	Item   *game.ItemInput `json:"item"`
	IDs    []string        `json:"ids"`
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

// requestResync asks the writer to enqueue a fresh snapshot (D-42). It is a
// flag rather than a queue on purpose: two requests arriving before the writer
// runs must coalesce into one snapshot.
func (c *client) requestResync() { c.wantsResync.Store(true) }

// setIdentity publishes this connection's resolved identity. It runs inside the
// engine's critical section, before the first enqueue of the handshake, so the
// fan-out of that same mutation already sees it.
func (c *client) setIdentity(role, playerID string) {
	c.who.Store(&identity{role: role, playerID: playerID})
}

// role is the connection's resolved role, or "" before a successful handshake.
func (c *client) role() string {
	if id := c.who.Load(); id != nil {
		return id.role
	}
	return ""
}

// playerID is the connection's player id: empty for screen, admin and any
// connection that has not joined.
func (c *client) playerID() string {
	if id := c.who.Load(); id != nil {
		return id.playerID
	}
	return ""
}

// snapshot builds this connection's own role projection from a lock-held view
// (D-25/D-26). It is the only way this package asks for a payload.
func (c *client) snapshot(v game.View) any {
	return v.Snapshot(game.Role(c.role()), c.playerID())
}

// describe names the connection in a log line without leaking a token.
func (c *client) describe() string {
	if id := c.playerID(); id != "" {
		return c.role() + " " + id
	}
	if role := c.role(); role != "" {
		return role
	}
	return "unjoined"
}

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
		// WR-01: bad_message covers two different failures, and the log-only
		// message must not conflate them. A JSON *syntax* error means the frame
		// is not JSON at all; a *type* error means a valid-JSON frame whose
		// field cannot be read as the catalog's type (a numeric admin field
		// sent as a float, say), which is a field-level problem the operator
		// should be able to see. The CODE stays the frozen bad_message (D-24).
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			c.sendError(CodeBadMessage, "message field "+typeErr.Field+" has the wrong type")
			return
		}
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
	if c.role() == "" && in.Type != TypeJoin {
		c.sendError(CodeNotJoined, "join first")
		return
	}

	switch in.Type {
	case TypeJoin:
		c.handleJoin(in)
	case TypeResync:
		// D-42: the snapshot is built under the engine's exclusive lock and
		// enqueued into this connection's own FIFO by the writer — never written
		// to the socket from here, so it cannot jump the queue, and coalesced so
		// two rapid requests do not queue two snapshots.
		c.requestResync()
	case TypeBidPlace:
		c.handleBid(in)
	case TypeAdminItemUpsert, TypeAdminItemDelete, TypeAdminItemReorder,
		TypeAdminItemStart, TypeAdminItemForceEnd, TypeAdminItemNext:
		c.handleAdmin(in)
	default:
		// D-24: the Phase 4–6 commands are already in the catalog, so a client
		// that sends one now gets a typed answer instead of silence.
		//
		// Deliberately outside D-23's rejectedJoins cap — that counter counts
		// join REJECTIONS, not answers — and so are the bad-JSON, no-type,
		// non-text and pre-join replies above. The 4 KiB read limit is the only
		// bound until the Phase 7 per-connection rate limit lands (CONTEXT
		// §Deferred).
		c.sendError(CodeUnsupported, "command not implemented in this phase")
	}
}

// handleBid routes one `bid.place` (D-52/D-53). The role gate comes first: a
// command from the wrong role is a protocol error and never reaches the engine
// (D-52 #1/H12/T-03-07). The amount itself is NOT pre-validated here — the
// ladder is the single validation surface, so `invalid_amount` travels to the
// bidder as the same typed `bid.rejected` every other refusal uses.
func (c *client) handleBid(in inbound) {
	if c.role() != RolePlayer {
		c.sendError(CodeBadMessage, "bid.place needs the player role")
		return
	}

	// D-52 #2 is a LADDER step, so the amount is parsed here and never
	// pre-refused (WR-01): a literal the decoder cannot read as a whole number
	// becomes 0, which the ladder answers with the frozen `invalid_amount`
	// reason — the copy the player is owed — instead of a frame-level
	// bad_message that would claim the frame was not JSON.
	//
	// D-53's refusal travels through this callback, which PlaceBid invokes with
	// the engine's exclusive lock held: the min_next_bid and budget it reports
	// are the numbers of that moment, and the private frame takes its seq on the
	// same lock-held FIFO as every state event (Pitfall 1).
	amount := amountFromLiteral(in.Amount)
	err := c.hub.engine.PlaceBid(c.playerID(), amount, func(v game.View, rejection game.BidRejection) {
		c.enqueue(v.Rev(), TypeBidRejected, rejection)
	})
	if err == nil {
		return
	}

	// A *BidRejectedError means the private frame already answered the player;
	// there is nothing left to say. Anything else is an engine-level failure.
	var rejected *game.BidRejectedError
	if errors.As(err, &rejected) {
		return
	}
	log.Printf("ws: bid from %s: %v", c.describe(), err)
	c.sendError(CodeBadMessage, "the bid could not be placed")
}

// amountFromLiteral reads a `bid.place` amount out of its raw JSON literal
// (D-52 #2/WR-01). ONLY a base-10 integer literal parses: a fraction (1.5), a
// whole number spelled with a decimal point (250000.0), an exponent (1e5), a
// quoted number, a bool, an array or an absent key all report 0 — and 0 can
// never be a valid bid, so the ladder refuses every one of them with the frozen
// `invalid_amount` reason instead of the decoder answering a frame-level
// bad_message that tells the player nothing.
//
// strconv returns a CLAMPED value alongside ErrRange, so the error is what
// decides; the parsed value is never used when err != nil.
func amountFromLiteral(raw json.RawMessage) int64 {
	amount, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
	if err != nil {
		return 0
	}
	return amount
}

// handleAdmin routes one `admin.item.*` command (D-64/D-65/D-66). Every command
// is role-gated before it can name a mutation, and every refusal is a private
// typed error that changes nothing (T-03-07): `not_authorized` for the wrong
// role, `bad_message` for a phase edge, an unknown id or an invalid item.
//
// The engine's own message is carried along for the log and for an operator
// reading a dev console — the UI maps the CODE to its copy and never renders
// the server's text (UI-SPEC §Copywriting).
func (c *client) handleAdmin(in inbound) {
	if c.role() != RoleAdmin {
		c.sendError(CodeNotAuthorized, "admin commands need the admin role")
		return
	}

	var err error
	switch in.Type {
	case TypeAdminItemUpsert:
		if in.Item == nil {
			c.sendError(CodeBadMessage, "admin.item.upsert needs an item object")
			return
		}
		err = c.hub.engine.ItemUpsert(*in.Item)
	case TypeAdminItemDelete:
		err = c.hub.engine.DeleteItem(in.ID)
	case TypeAdminItemReorder:
		err = c.hub.engine.ReorderItems(in.IDs)
	case TypeAdminItemStart:
		err = c.hub.engine.StartItem(in.ID)
	case TypeAdminItemForceEnd:
		err = c.hub.engine.ForceEnd()
	case TypeAdminItemNext:
		err = c.hub.engine.NextItem()
	}

	if err != nil {
		c.sendError(CodeBadMessage, err.Error())
	}
}

// handleJoin completes D-23's handshake for all three roles. The whole
// handshake — identity resolution, registry supersede, hello and the role
// snapshot — runs inside ONE engine.mu critical section, which is what makes
// "no event can slip between hello and the snapshot" a structural property
// rather than a hope.
//
// The admin role is authorized here first (D-76/ADMN-04): the check cannot run
// at the upgrade, because the role does not exist until the first frame, so the
// cookie captured by ServeWS and the frame's own `key` are both tested before
// the engine is asked to resolve anything.
func (c *client) handleJoin(in inbound) {
	if c.role() != "" {
		c.sendError(CodeUnsupported, "this connection has already joined")
		return
	}
	switch in.Role {
	case RolePlayer, RoleScreen, RoleAdmin:
	default:
		c.sendError(CodeBadMessage, "join needs role player|screen|admin")
		return
	}

	// D-76: cookie OR join.key, compared in constant time against the boot key.
	// A refusal reuses the ordinary rejection path — the typed error goes out on
	// the lock-held private FIFO, the socket stays open for a corrected attempt,
	// and D-23's cap retires it — so an unauthorized admin join is
	// indistinguishable, structurally, from a player who mistyped a nickname.
	if in.Role == RoleAdmin && !c.hub.authorizedAdmin(c.adminCookie, in.Key) {
		c.sendError(CodeNotAuthorized, "admin join needs the admin key")
		c.rejectedJoins++
		if c.rejectedJoins >= maxRejectedJoins {
			c.retire()
		}
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
	c.setIdentity(string(hello.Role), hello.PlayerID)

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

// drain discards everything already queued. The messages belong to a burst this
// connection can no longer reconstruct, and the snapshot that follows
// supersedes them (D-41).
func (c *client) drain() {
	for {
		select {
		case <-c.send:
			c.queued.Add(-1)
		default:
			return
		}
	}
}

// enqueue hands one message to this connection's FIFO. It never blocks and
// never closes the channel.
//
// PRECONDITION: engine.mu is held exclusively (D-33). That is what makes
// c.seq++ and the send race-free: the registry read is lock-free, but no two
// enqueues for the same client can overlap only because every producer is
// inside the engine's critical section.
func (c *client) enqueue(rev int64, typ string, data any) {
	c.enqueueLocked(rev, typ, data, false)
}

// enqueueLocked is enqueue with the fence bypassed for the recovery snapshot.
// recovery also skips re-arming the fence: it is the message that ends the
// episode.
func (c *client) enqueueLocked(rev int64, typ string, data any, recovery bool) bool {
	next := c.seq + 1

	// D-41: while the fence is up this connection is owed a snapshot, so every
	// later message is suppressed rather than delivered as half a burst. seq is
	// still consumed, which is what makes the gap visible to the client.
	if !recovery && c.fence.Load() {
		c.seq = next
		c.suppressed.Add(1)
		return false
	}

	payload, err := json.Marshal(data)
	if err != nil {
		// A projection that cannot be serialized is a projector bug, not a
		// client error. The seq is still consumed so the client sees the gap.
		c.seq = next
		log.Printf("ws: marshaling %s: %v", typ, err)
		return false
	}

	c.seq = next

	select {
	case c.send <- Envelope{Type: typ, Seq: next, Rev: rev, TS: c.hub.now().UnixMilli(), Data: payload}:
		c.queued.Add(1)
		return true
	default:
		c.drops.Add(1)
		c.hub.drops.Add(1)
		if !recovery && !c.fence.Swap(true) {
			// Logged once per episode: the fence keeps every later drop from
			// coming back through here (research: drops logged at most once).
			log.Printf("ws: %s fell behind; scheduling a resync snapshot", c.describe())
		}
		return false
	}
}

// sendError answers this connection with the frozen typed error (D-24). It
// delegates to the hub's lock-held producer: the read pump must never touch this
// connection's seq counter or FIFO itself, or a private error could race the
// mutation fan-out into a duplicate seq that the client's gate would then
// discard as stale (D-21/D-33, GAP 1).
//
// A private answer is not a state event: it carries the current revision and
// never bumps it (D-35).
func (c *client) sendError(code, message string) {
	c.hub.sendError(c, code, message)
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
		// D-41: recovery runs here, in the only goroutine that could have made
		// room in the FIFO, and never on the producer's path.
		if c.fence.Load() {
			c.hub.recover(c)
			continue
		}
		if c.wantsResync.Load() {
			c.hub.applyResync(c)
			continue
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
