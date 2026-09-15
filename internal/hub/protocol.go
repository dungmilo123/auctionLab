// Package hub owns the WebSocket surface: the frozen wire protocol, the
// per-connection policies and (from plan 02-03) the registry and fan-out.
//
// # Why seq and rev are two different numbers
//
// rev is the GAME revision: it increments exactly once per successful,
// persisted mutation, and it is what a client uses to decide whether a
// snapshot is newer than its current state.
//
// seq is this CONNECTION's stream sequence: it increments per message actually
// handed to the socket, starting at 1 with hello, and it is what a client uses
// to detect duplicates and drops.
//
// These are NOT the same contract. One mutation legitimately produces several
// messages (phase.changed + item.intro + leaderboard; item.settled +
// leaderboard), and tick/presence messages carry the current rev without
// bumping it. If a single token served both roles, a client following
// "ignore seq <= current" would silently discard every sibling message of that
// revision — the leaderboard would never update and the reveal would never
// arrive on screen. (Research: .planning/research/PITFALLS.md Pitfall 3.)
//
// DO NOT collapse these two fields. Add fields and types; never rename,
// repurpose, or remove them — this file is frozen at the end of Phase 2 (D-24),
// and every later client, loadtest bot and test binds to it.
package hub

import "encoding/json"

// Envelope is the frozen wire frame (D-21). Every server→client message is one
// of these, and the five JSON keys are the contract — not the Go field names.
type Envelope struct {
	Type string          `json:"type"`
	Seq  int64           `json:"seq"`  // per-connection stream sequence, 1 at hello (D-21)
	Rev  int64           `json:"rev"`  // game revision, 0 in LOBBY before the first mutation (D-35)
	TS   int64           `json:"ts"`   // server clock, milliseconds (injected clock, D-38)
	Data json.RawMessage `json:"data"` // the message payload
}

// Connection roles. The role is chosen once, in the join (D-23).
const (
	RolePlayer = "player"
	RoleScreen = "screen"
	RoleAdmin  = "admin"
)

// Message types — the complete catalog, defined NOW even where the behaviour
// lands in a later phase (D-24). An unimplemented command answers
// error{code: CodeUnsupported} rather than looking like an unknown message.
const (
	// client → server
	TypeJoin         = "join"
	TypeResync       = "resync"
	TypeBidPlace     = "bid.place"     // Phase 3
	TypeDutchAccept  = "dutch.accept"  // Phase 5
	TypeSealedSubmit = "sealed.submit" // Phase 6

	// admin.* (Phase 3/4)
	TypeAdminItemUpsert   = "admin.item.upsert"
	TypeAdminItemDelete   = "admin.item.delete"
	TypeAdminItemReorder  = "admin.item.reorder"
	TypeAdminItemStart    = "admin.item.start"
	TypeAdminItemForceEnd = "admin.item.force_end"
	TypeAdminItemNext     = "admin.item.next"
	TypeAdminGameReset    = "admin.game.reset"
	TypeAdminConfigUpdate = "admin.config.update"

	// server → client
	TypeHello          = "hello"
	TypeStateSnapshot  = "state.snapshot"
	TypePhaseChanged   = "phase.changed"
	TypeItemIntro      = "item.intro"      // Phase 3
	TypeItemLive       = "item.live"       // Phase 3
	TypePriceTick      = "price.tick"      // Phase 3
	TypeBidAccepted    = "bid.accepted"    // Phase 3
	TypeBidRejected    = "bid.rejected"    // Phase 3
	TypeSealedReceived = "sealed.received" // Phase 6
	TypeItemSettled    = "item.settled"    // Phase 3
	TypeLeaderboard    = "leaderboard"     // Phase 3
	TypeFinale         = "finale"          // Phase 4
	TypeTunnelStatus   = "tunnel.status"   // Phase 7 — CONTROL PLANE, see below
	TypeError          = "error"
)

// Error codes. Additions are allowed; renames are not (D-24).
const (
	CodeNicknameInvalid = "nickname_invalid"
	CodeNicknameTaken   = "nickname_taken"
	CodeTokenInvalid    = "token_invalid"
	CodeNotJoined       = "not_joined"
	CodeBadMessage      = "bad_message"
	CodeUnsupported     = "unsupported"
	CodeNotAuthorized   = "not_authorized" // wired in Phase 4 (ADMN-04)
)

// tunnel.status is the one control-plane message in this catalog. It is
// orthogonal to the game revision space: a client must apply it regardless of
// seq and must NOT feed it through the revision gate — otherwise a tunnel URL
// rotation whose rev looked stale would be dropped exactly when the venue needs
// it (Phase 7 owns the behaviour; the exception is recorded here now so a later
// reader cannot "simplify" it away).
const tunnelStatusControlPlane = TypeTunnelStatus

// errorPayload is the typed error body (D-24).
type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// helloPayload is D-23's hello. Player identity (id and the D-29 token) arrives
// with 02-02/02-03, when the engine owns identity: D-24 allows adding optional
// fields, never renaming or removing one.
type helloPayload struct {
	Role string `json:"role"`
	Rev  int64  `json:"rev"`
}

// snapshotPayload is the skeleton role snapshot. D-25's self-sufficient
// projection (me, lobby, leaderboard, server time) lands with 02-02's role
// projections; what exists at this point in the phase is the phase and the
// revision, and both already travel in the frame.
type snapshotPayload struct {
	Role  string `json:"role"`
	Phase string `json:"phase"`
	Rev   int64  `json:"rev"`
}
