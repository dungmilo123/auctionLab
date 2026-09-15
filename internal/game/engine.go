package game

import (
	"sync"
	"sync/atomic"
	"time"

	"auctionlab/internal/store"
)

// This file is the engine: the single mutation path (D-34), the one canonical
// revision (D-35) and presence (D-37).
//
// RED contract declaration: the exported surface below is what engine_test.go
// and (in the hub) the fan-out bind to. Every body lands with the GREEN commit.

// Role is one client role (D-23). The values are the frozen wire strings from
// internal/hub/protocol.go; game cannot import hub (hub imports game), so the
// three literals are declared here as well and pinned on both sides — by
// identity_test.go here and by the hub's protocol test.
type Role string

const (
	RolePlayer Role = "player"
	RoleScreen Role = "screen"
	RoleAdmin  Role = "admin"
)

// Rejection codes are D-24's frozen error catalog, under the same duplication
// note as Role above.
const (
	CodeNicknameInvalid = "nickname_invalid"
	CodeNicknameTaken   = "nickname_taken"
	CodeTokenInvalid    = "token_invalid"
)

// JoinError is a rejection the caller answers with error{code, message}; the
// socket stays open so the client can retry on the same connection (D-23).
type JoinError struct {
	Code    string
	Message string
}

func (e *JoinError) Error() string { return e.Code + ": " + e.Message }

// JoinRequest is one client's join attempt (D-23). Key is accepted and ignored
// in Phase 2: the ADMIN_KEY check is Phase 4 (ADMN-04) and no secret exists yet.
type JoinRequest struct {
	Role     Role
	Nickname string
	Token    string
	Key      string
}

// JoinResult is what the caller needs to answer the connection: the identity
// (player id and the one token this connection may ever see, D-29) and the
// revision the handshake ran at.
type JoinResult struct {
	Role     Role
	PlayerID string
	Token    string
	Resumed  bool
	Rev      int64
}

// Hello is the private handshake message for one connection (D-23). It is the
// only message in the protocol that ever carries a token, and only the token of
// the player it is addressed to (D-26/D-29).
type Hello struct {
	Role     Role   `json:"role"`
	PlayerID string `json:"player,omitempty"`
	Nickname string `json:"nickname,omitempty"`
	Token    string `json:"token,omitempty"`
	Rev      int64  `json:"rev"`
}

// View is a read-only window on the committed state, valid only while the
// engine's exclusive lock is held — inside a Publisher call or a Section
// callback. It is how the hub builds one role projection per client without
// ever taking a lock of its own (D-33).
type View interface {
	Rev() int64
	Snapshot(role Role, playerID string) any
}

// Publisher is the engine's fan-out edge. The engine calls it with the
// exclusive lock held, after a mutation or presence change has been applied and
// persisted; implementations must not block and must not do socket I/O — the
// hub's implementation enqueues into per-client channels (D-33/D-41).
type Publisher func(rev int64, v View)

// Config wires the engine's dependencies. Store, Now and Fatal are injected so
// tests can pin timestamps (D-38) and observe commit failures (D-34).
type Config struct {
	Store     *store.Store
	Now       func() time.Time
	Fatal     func(error)
	Publisher Publisher
}

// engineConfig mirrors the persisted game.config_json (D-36).
type engineConfig struct {
	StartingBudget int64  `json:"starting_budget"`
	Currency       string `json:"currency"`
}

// player is one player's authoritative row plus its live presence. The struct
// is unexported and carries no JSON tags: it can never be serialized to a
// client (D-26).
type player struct {
	id          string
	nickname    string
	nicknameKey string
	token       string
	budget      int64
	totalValue  int64
	joinedAt    int64
	lastSeenAt  int64
}

// item mirrors the `items` table (PLAN.md §5). Phase 2 never loads items — the
// item flow is Phase 3 — but truth_value lives here so the projection boundary
// is real and testable today (D-26's positive control).
type item struct {
	id          string
	orderIndex  int64
	name        string
	description string
	auctionType string
	truthValue  int64
	hintMin     int64
	hintMax     int64
	startPrice  int64
	status      string
}

// sealedBid mirrors one `sealed_bids` row (PLAN.md §5). Phase 6 owns the flow;
// the field exists now because the leak boundary must be proven against a
// populated fixture, not against an empty one (D-26).
type sealedBid struct {
	itemID      string
	playerID    string
	amount      int64
	submittedAt int64
	updatedAt   int64
}

// state is the authoritative in-memory game state. Every field is unexported
// and there are no JSON tags: the only way out of this package is project.go's
// role projections (D-26).
type state struct {
	phase         Phase
	revision      int64
	currentItemID string
	config        engineConfig

	players   map[string]*player
	order     []string // player ids in join order
	connected map[string]bool

	items      map[string]*item
	itemOrder  []string
	sealedBids []sealedBid
}

// Engine is the authoritative game state and the single mutation path. All
// mutation goes through Apply (D-34); reads go through Snapshot or Section.
type Engine struct {
	mu sync.RWMutex

	state    *state
	revision atomic.Int64

	store *store.Store
	now   func() time.Time
	fatal func(error)

	publisher atomic.Pointer[Publisher]
}

// New loads the persisted state and returns the engine (D-39). A missing game
// row is created by the bootstrap first; a load error is fatal rather than a
// silently empty lobby.
func New(cfg Config) (*Engine, error) {
	return nil, errPendingImplementation
}

// SetPublisher installs the fan-out edge. It is called once at boot, before the
// server accepts connections (the hub needs the engine to exist first, and the
// engine needs the hub to fan out — hence a setter rather than a Config field
// only).
func (e *Engine) SetPublisher(p Publisher) {}

// Rev is the one canonical game revision (D-35), readable without the lock.
func (e *Engine) Rev() int64 { return e.revision.Load() }

// Phase is the current phase of the PLAN.md §6 machine.
func (e *Engine) Phase() Phase { return "" }

// Apply is the only game-mutation path (D-34): validate against the scratch
// copy, mutate it, persist the write set together with game.revision in one
// transaction, then publish — all inside one exclusive lock section, so no
// event can ever be enqueued out of order (PLAN §3.1 #5).
func (e *Engine) Apply(fn func(s *state) error) error {
	return errPendingImplementation
}

// Join resolves one client's join (D-23/D-30) under the exclusive lock. emit,
// when non-nil, is called inside the same critical section with hello first and
// the role snapshot second, so no event can slip between them.
func (e *Engine) Join(req JoinRequest, emit func(Hello, any)) (JoinResult, error) {
	return JoinResult{}, errPendingImplementation
}

// Connect marks a player connected and announces it. Presence is in-memory
// only and never bumps the revision (D-37); last_seen_at is persisted.
func (e *Engine) Connect(playerID string) error {
	return errPendingImplementation
}

// Disconnect marks a player disconnected and announces it (D-37). It is a
// no-op for an unknown player: teardown must never depend on the row existing.
func (e *Engine) Disconnect(playerID string) error {
	return errPendingImplementation
}

// Section runs fn with the exclusive lock held. It is the hub's way to build a
// resync snapshot and enqueue it with no producer able to interleave (D-42).
func (e *Engine) Section(fn func(v View) error) error {
	return errPendingImplementation
}

// Snapshot builds one client's role projection under a read lock, for callers
// that are not already inside a Section.
func (e *Engine) Snapshot(role Role, playerID string) any {
	return nil
}

func (e *Engine) snapshotLocked(role Role, playerID string) any {
	return nil
}

func (e *Engine) publishLocked() {}

func (e *Engine) applyLocked(fn func(s *state) error, bumpRevision bool, emit func(View)) error {
	return errPendingImplementation
}

// clone deep-copies the state so a failed validation can never leave the live
// state half-mutated.
func (s *state) clone() *state { return nil }

func (s *state) playerRows() []store.PlayerRow          { return nil }
func (s *state) playerByToken(token string) *player     { return nil }
func (s *state) playerByNicknameKey(key string) *player { return nil }
