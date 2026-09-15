// Package hub owns the WebSocket surface: the frozen wire protocol, the
// per-connection policies and the connection registry with its fan-out.
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
//
// D-21 also supersedes PLAN.md §8's original envelope wording, which used a
// single `seq` that WAS the game revision. §8's message and error tables still
// describe this protocol faithfully; its "seq = revision" reading does not.
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

// # D-24 freeze (final, end of Phase 2)
//
// Everything below is FROZEN. Later phases may add message types, error codes
// and optional payload fields. Later phases may NOT rename, remove or repurpose
// a field, a type or a code, and may not change what a frozen value means on the
// wire.
//
// The freeze is not a comment: it is enforced by two tests, and any change to
// the catalog has to pass through them deliberately.
//
//   - protocol_test.go#TestProtocol compares every constant against the literal
//     it publishes, so a renamed identifier or a changed value fails there.
//   - protocol_test.go#TestProtocolFreeze compares the inventory below against
//     the pinned literal list, so a removal fails and an addition has to be
//     registered here rather than arriving silently.
//
// Adding a type is therefore a two-line edit (append the constant, append it
// here) plus the test's pinned list — which is exactly the "additive only"
// contract D-24 promises the client and every later phase.
//
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

// frozenCatalog is the D-24 freeze made checkable: every message type this
// protocol publishes, client→server first, then server→client, in catalog
// order.
//
// It is an array rather than a slice so nothing in the package can append to it
// by accident. The protocol_test.go freeze test asserts it equals the pinned
// literal list exactly: a rename or a removal fails there, and an addition —
// the one edit D-24 permits — has to be appended here and in the test, which is
// what makes a wider wire surface a deliberate act instead of a silent one.
//
// Later phases APPEND. They never reorder and never delete.
var frozenCatalog = [...]string{
	// client → server
	TypeJoin,
	TypeResync,
	TypeBidPlace,
	TypeDutchAccept,
	TypeSealedSubmit,
	TypeAdminItemUpsert,
	TypeAdminItemDelete,
	TypeAdminItemReorder,
	TypeAdminItemStart,
	TypeAdminItemForceEnd,
	TypeAdminItemNext,
	TypeAdminGameReset,
	TypeAdminConfigUpdate,
	// server → client
	TypeHello,
	TypeStateSnapshot,
	TypePhaseChanged,
	TypeItemIntro,
	TypeItemLive,
	TypePriceTick,
	TypeBidAccepted,
	TypeBidRejected,
	TypeSealedReceived,
	TypeItemSettled,
	TypeLeaderboard,
	TypeFinale,
	TypeTunnelStatus,
	TypeError,
}

// frozenErrorCodes is the same freeze for the rejection codes: append-only, and
// pinned literally by protocol_test.go#TestProtocolFreeze.
var frozenErrorCodes = [...]string{
	CodeNicknameInvalid,
	CodeNicknameTaken,
	CodeTokenInvalid,
	CodeNotJoined,
	CodeBadMessage,
	CodeUnsupported,
	CodeNotAuthorized,
}

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

// The handshake and role-snapshot payloads are NOT declared here: the wire
// payloads are game.Hello and the three role projections in
// internal/game/project.go, which this package enqueues verbatim. 02-01's
// skeleton stand-ins were removed at the freeze so this file describes only the
// contract that is actually on the wire.
