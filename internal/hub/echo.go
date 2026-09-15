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

	"auctionlab/internal/game"
)

// This file is the Phase-2 protocol skeleton. Plan 02-03 Task 1 replaces it
// wholesale with the real Hub/Client pair (registry, drop/resync, supersede);
// until then `Echo` stays exported because cmd/server/main.go registers it at
// /ws — the Phase 1 echo behaviour it was named for is gone (D-01: Phase 2
// replaces the temporary echo handler wholesale).

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

// Echo upgrades GET /ws and serves the protocol skeleton: a join is answered by
// hello (seq 1) followed by the role snapshot (seq 2), and every other frame by
// the frozen catalog's answer — never by an echo.
func Echo(w http.ResponseWriter, r *http.Request) {
	serve(w, r, DefaultPolicy)
}

// serve is Echo with the D-40 policy injected, so tests can pin the keepalive
// mechanism and the frame cap with timings short enough to observe (the WR-01
// handoff: Phase 1's numbers were placeholders and are replaced here).
func serve(w http.ResponseWriter, r *http.Request, policy Policy) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response.
		logUpgradeFailure(err)
		return
	}
	defer func() { _ = conn.Close() }()

	// Liveness (D-40 / threat T-02-03): a peer that vanishes without a close
	// frame — phone suspended, Wi-Fi drop, NAT rebind — would otherwise hold
	// this goroutine and its file descriptor until the kernel's TCP keep-alive
	// gives up. The read deadline is the enforcement; the pong handler is what
	// spares a healthy but silent peer from it.
	conn.SetReadLimit(policy.ReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(policy.PongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(policy.PongWait))
	})

	// Ping from a single ticker goroutine: WriteControl is the one write
	// gorilla allows concurrently with the envelope writer below.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(policy.PingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(policy.WriteWait)); err != nil {
					// Unreachable peer: close so the blocked read returns now
					// instead of at the deadline.
					_ = conn.Close()
					return
				}
			}
		}
	}()

	s := &session{
		conn:   conn,
		policy: policy,
		now:    time.Now,
		rev:    0, // D-36: the bootstrapped game row is LOBBY at revision 0; 02-02's engine owns this
	}

	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			// The catalog is JSON text; anything else is malformed input and
			// gets the typed answer instead of silence (T-02-05).
			s.sendError(CodeBadMessage, "only text frames carry protocol messages")
			continue
		}
		s.handle(payload)
	}
}

// session is one connection's protocol state.
//
// seq is per-connection and starts at 1 with hello (D-21); rev is the game
// revision every frame of this connection carries, and this skeleton has only
// the bootstrapped value (D-35/D-36). role is empty until a successful join,
// which is what D-23's not_joined guard keys on.
type session struct {
	conn   *websocket.Conn
	policy Policy
	now    func() time.Time

	seq  int64
	rev  int64
	role string
}

// inbound is the union of the client→server fields Phase 2 needs to route a
// frame. Unknown fields are ignored on purpose: D-24 freezes the catalog, not
// the payload structs, and later phases add optional fields to the same
// commands (D-24's additive rule).
type inbound struct {
	Type     string `json:"type"`
	Role     string `json:"role"`
	Nickname string `json:"nickname"` // consumed by 02-02's identity resolution
	Token    string `json:"token"`    // consumed by 02-02's identity resolution
	Key      string `json:"key"`      // ADMIN_KEY check is Phase 4 (D-24: accepted unchecked for now)
}

// handle routes one text frame against the frozen catalog.
func (s *session) handle(payload []byte) {
	var in inbound
	if err := json.Unmarshal(payload, &in); err != nil {
		s.sendError(CodeBadMessage, "message is not valid JSON")
		return
	}
	if in.Type == "" {
		s.sendError(CodeBadMessage, "message has no type")
		return
	}

	// D-23: every command except join is refused before a successful join — and
	// the socket stays open, so the client can fix its nickname and retry
	// without a reload.
	if s.role == "" && in.Type != TypeJoin {
		s.sendError(CodeNotJoined, "join first")
		return
	}

	switch in.Type {
	case TypeJoin:
		s.handleJoin(in)
	case TypeResync:
		// D-42: 02-03 enqueues this snapshot into the connection's own FIFO;
		// the skeleton writes it back on the same stream, which is what makes
		// the ordering the client sees identical either way.
		s.sendSnapshot()
	default:
		// D-24: the Phase 3–6 commands are already in the catalog, so a client
		// that sends one now gets a typed answer instead of silence.
		s.sendError(CodeUnsupported, "command not implemented in this phase")
	}
}

// handleJoin completes D-23's handshake for all three roles.
//
// The skeleton has no engine lock yet, so hello and the snapshot are written
// back to back; 02-03's Hub performs exactly this pair inside one engine.mu
// critical section, which is what turns the ordering this tracer pins into a
// guarantee (D-23: registration + hello + snapshot share one critical section).
func (s *session) handleJoin(in inbound) {
	if s.role != "" {
		s.sendError(CodeUnsupported, "this connection has already joined")
		return
	}
	switch in.Role {
	case RolePlayer, RoleScreen, RoleAdmin:
	default:
		s.sendError(CodeBadMessage, "join needs role player|screen|admin")
		return
	}

	s.role = in.Role
	s.send(TypeHello, helloPayload{Role: s.role, Rev: s.rev})
	s.sendSnapshot()
}

// send stamps the next per-connection seq and writes one envelope. It is the
// only application writer on this socket; the ping goroutine above uses
// WriteControl, which gorilla documents as the one safe concurrent write.
func (s *session) send(kind string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("ws: marshaling %s: %v", kind, err)
		return
	}

	s.seq++
	env := Envelope{Type: kind, Seq: s.seq, Rev: s.rev, TS: s.now().UnixMilli(), Data: payload}

	_ = s.conn.SetWriteDeadline(time.Now().Add(s.policy.WriteWait))
	if err := s.conn.WriteJSON(env); err != nil {
		// The read loop owns teardown; closing here just guarantees this
		// connection cannot write a half-sent stream.
		_ = s.conn.Close()
	}
}

// sendError writes the frozen typed error (D-24).
func (s *session) sendError(code, message string) {
	s.send(TypeError, errorPayload{Code: code, Message: message})
}

// sendSnapshot writes the role snapshot — the sync point of D-22's client
// reconciliation rules, and the second frame of a handshake.
func (s *session) sendSnapshot() {
	s.send(TypeStateSnapshot, snapshotPayload{
		Role:  s.role,
		Phase: string(game.PhaseLobby),
		Rev:   s.rev,
	})
}
