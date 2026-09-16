// Package game owns the authoritative game state: the phase machine (state.go),
// the single mutation path, presence, and the role projections (project.go).
//
// The rules this package exists to enforce:
//
//   - One mutation path. Everything that changes game state goes through
//     Engine.Apply, which validates against a scratch copy of the state,
//     persists the write set and game.revision in ONE transaction, and only
//     then publishes — all while holding the exclusive lock (D-34/D-35,
//     PLAN.md §3.1 #5).
//   - One revision. `game.revision` is the only counter; it moves on a
//     successful game mutation and never for presence, a tick or a private
//     error (D-35).
//   - The internal state never leaves this package. Every field of `state` is
//     unexported and has no JSON tags; the only way out is a role projection
//     (D-25/D-26).
package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"auctionlab/internal/store"
)

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

// JoinRequest is one client's join attempt (D-23). Key is D-76's script/probe
// capability, and the ENGINE deliberately ignores it: the ADMIN_KEY check runs
// in the hub before this call, because a capability that reached game state
// would be one projection field away from a browser (D-74/ADMN-04).
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
	// Tick builds the D-56 timing frame, or reports false when no open window
	// exists (not LIVE, or no run). It reads the same lock-held state the
	// mutation just committed.
	Tick() (PriceTick, bool)
}

// Publisher is the engine's fan-out edge. The engine calls it with the
// exclusive lock held, after a mutation or presence change has been applied and
// persisted; implementations must not block and must not do socket I/O — the
// hub's implementation enqueues into per-client channels (D-33/D-41).
//
// events is the exact ordered list the committed mutation recorded on the state
// (D-57): the hub expands it into typed frames on this same lock-held path. An
// empty list means the mutation produced no typed frame of its own (a join, a
// presence change), and the role snapshot is the frame (D-33/H2).
type Publisher func(rev int64, v View, events []Event)

// Config wires the engine's dependencies. Store, Now and Fatal are injected so
// tests can pin timestamps (D-38) and observe commit failures (D-34).
type Config struct {
	Store     *store.Store
	Now       func() time.Time
	Fatal     func(error)
	Publisher Publisher

	// JoinURLCandidates is D-83's boot-time LAN enumeration: the join URLs the
	// admin panel offers for selection, in rank order. It is a boot input and
	// never durable state — config_json persists the SELECTED url (D-84), not
	// the proposals — so it is passed here rather than loaded from the store.
	JoinURLCandidates []string
}

// engineConfig mirrors the persisted game.config_json (D-36). Persisting the
// defaults means a future admin.config.update changes this row, not a constant.
//
// Phase 4 (D-84, additive): `JoinURL` is the persisted SELECTION, and it is
// omitempty so every file written before Phase 4 keeps its exact shape (the key
// is simply absent, which unmarshals to ""). It is also the reason the ADMIN_KEY
// can never be persisted here: config_json is projected to the admin panel, so a
// secret placed in it would be one `AdminConfig` field away from the browser
// (D-74).
type engineConfig struct {
	StartingBudget int64  `json:"starting_budget"`
	Currency       string `json:"currency"`
	JoinURL        string `json:"join_url,omitempty"`
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

// item mirrors the `items` table (PLAN.md §5). It is a flat, comparable value
// struct on purpose: the write set is diff-based, so `*a != *b` is the entire
// "this row changed" test (H3). Every nullable numeric column is normalized to 0
// at load, and 0 is written back as SQL NULL by the store.
type item struct {
	id                  string
	orderIndex          int64
	name                string
	description         string
	imageURL            string
	auctionType         string
	truthValue          int64
	hintMin             int64
	hintMax             int64
	startPrice          int64
	minIncrement        int64
	floorPrice          int64
	decrementStep       int64
	decrementIntervalMS int64
	durationMS          int64
	antiSnipeMode       string
	antiSnipeWindowMS   int64
	maxExtensions       int64 // 0 = no cap (NULL in the table)
	status              string
}

// sealedBid mirrors one `sealed_bids` row (PLAN.md §5). Phase 6 owns the flow;
// the type exists now because the leak boundary must be proven against a
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

	// Phase 3 (D-49): the live auction. currentAuction is nil outside a run;
	// bids is the process-wide append-only trail the projections and settle
	// derive from (the run boundary is startedAt).
	currentAuction *auctionRun
	bids           []bid

	// events is what the mutation that produced THIS state emitted, in order.
	// It is not durable state and clone() never copies it: it exists so the
	// lock-held fan-out edge (publishLocked) hands the hub the exact frames the
	// committed mutation produced (D-57).
	events []Event

	// reset is the D-78 reset intent of the mutation that produced THIS state
	// (the mode, or "" for every other mutation). It is per-mutation intent
	// rather than durable state, and clone() never copies it — for the same
	// reason `events` is not copied. It exists because a reset's write set
	// cannot be derived from a diff: the trail collapses to nothing, sealed bids
	// have no in-memory identity at all until Phase 6, and a nil currentAuction
	// means the run is over for the ordinary item flow too, not that the
	// auction_runtime rows must go.
	reset ResetMode

	// joinURLCandidates is D-83's boot-time enumeration: the LAN addresses this
	// show can be reached on, in rank order. It is NOT durable — config_json
	// persists only the selected URL (D-84) — and it never changes after New,
	// so clone() may copy the slice header rather than the strings.
	joinURLCandidates []string
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

	// publisher is set once at boot (SetPublisher) and read on every commit
	// path under the engine lock; the atomic keeps a late install race-free for
	// tests that wire the edge after New.
	publisher atomic.Pointer[Publisher]
}

// New loads the persisted state and returns the engine (D-39). The boot order
// is Bootstrap → LoadGame → LoadPlayers → the auction half (D-62): items, the
// runtime row and the bid trail are rebuilt, the phase↔item↔runtime triple is
// validated, and a deadline that passed while the process was down settles HERE
// — before main builds the mux, before the hub exists and long before Listen.
// Every load error is fatal rather than a silently empty lobby.
func New(cfg Config) (*Engine, error) {
	if cfg.Store == nil {
		return nil, errors.New("game: Config.Store is required")
	}
	if cfg.Fatal == nil {
		return nil, errors.New("game: Config.Fatal is required — a failed commit must fail hard (D-34)")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	e := &Engine{store: cfg.Store, now: now, fatal: cfg.Fatal}
	if cfg.Publisher != nil {
		e.SetPublisher(cfg.Publisher)
	}

	if err := store.Bootstrap(cfg.Store.DB, now()); err != nil {
		return nil, err
	}
	gameRow, err := cfg.Store.LoadGame()
	if err != nil {
		return nil, err
	}
	rows, err := cfg.Store.LoadPlayers()
	if err != nil {
		return nil, err
	}

	var ec engineConfig
	if err := json.Unmarshal([]byte(gameRow.ConfigJSON), &ec); err != nil {
		return nil, fmt.Errorf("game: parsing the persisted config_json %q: %w", gameRow.ConfigJSON, err)
	}
	if ec.StartingBudget <= 0 {
		return nil, fmt.Errorf("game: persisted starting_budget is %d, want a positive number", ec.StartingBudget)
	}
	if ec.Currency == "" {
		return nil, errors.New("game: persisted currency is empty")
	}
	if !slices.Contains(AllPhases(), Phase(gameRow.Phase)) {
		return nil, fmt.Errorf("game: persisted phase %q is not part of the PLAN.md §6 machine", gameRow.Phase)
	}

	s := &state{
		phase:         Phase(gameRow.Phase),
		revision:      gameRow.Revision,
		currentItemID: gameRow.CurrentItemID,
		config:        ec,
		players:       make(map[string]*player, len(rows)),
		order:         make([]string, 0, len(rows)),
		connected:     make(map[string]bool, len(rows)),
		items:         make(map[string]*item),
		// D-83: the boot-time candidate list. A defensive copy, so a caller that
		// reuses its slice cannot mutate the engine's proposals afterwards.
		joinURLCandidates: append([]string(nil), cfg.JoinURLCandidates...),
	}
	for _, row := range rows {
		s.players[row.ID] = &player{
			id:          row.ID,
			nickname:    row.Nickname,
			nicknameKey: row.NicknameKey,
			token:       row.Token,
			budget:      row.Budget,
			totalValue:  row.TotalValue,
			joinedAt:    row.JoinedAt,
			lastSeenAt:  row.LastSeenAt,
		}
		s.order = append(s.order, row.ID)
	}

	e.state = s
	e.revision.Store(s.revision)

	// Phase 3 (D-62): the auction half of the resume. Loading, validating and —
	// when the wall clock already passed the deadline — settling all happen
	// before this function returns, so no caller can observe a LIVE auction
	// that is due (RT-05).
	if err := e.loadAuctionState(); err != nil {
		return nil, err
	}
	if err := e.settleDueAtBoot(); err != nil {
		return nil, err
	}

	// D-37: a restart starts with every player disconnected. Presence is
	// in-memory state, and nobody can be connected to a process that has not
	// started.
	return e, nil
}

// SetPublisher installs the fan-out edge. It is called once at boot, before the
// server accepts connections: the hub needs the engine to construct itself, and
// the engine needs the hub to fan out, so the edge is installed rather than
// passed to New.
func (e *Engine) SetPublisher(p Publisher) { e.publisher.Store(&p) }

// Rev is the one canonical game revision (D-35), readable without the lock.
func (e *Engine) Rev() int64 { return e.revision.Load() }

// Phase is the current phase of the PLAN.md §6 machine.
func (e *Engine) Phase() Phase {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state.phase
}

// PlayerCount is the number of identities the boot loaded. It exists for the
// boot log (D-39: report what was loaded) and is cheap enough for a health
// endpoint later.
func (e *Engine) PlayerCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.state.players)
}

// Apply is the only game-mutation path (D-34): the mutation runs against a
// scratch copy, the resulting write set is persisted together with
// game.revision in ONE transaction, and only then is the new state published in
// memory and fanned out — all inside one exclusive lock section, so no event
// can ever be enqueued out of order (PLAN.md §3.1 #5).
//
// The parameter type is unexported on purpose: the mutation surface is this
// package's business, and no caller outside it can name the state it edits
// (D-26).
func (e *Engine) Apply(fn func(s *state) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.applyLocked(fn, true, nil)
}

// applyLocked is Apply with the lock already held, so Join and the presence
// calls share the exact same one-mutation-path instead of growing a second one
// (D-34). bumpRevision is true only for game mutations; presence changes ride
// the current revision (D-37). emit, when non-nil, runs after the state swap
// and before the fan-out, which is what makes D-23's hello-then-snapshot pair
// atomic.
func (e *Engine) applyLocked(fn func(s *state) error, bumpRevision bool, emit func(View)) error {
	next := e.state.clone()
	if err := fn(next); err != nil {
		return err // nothing persisted, nothing published
	}
	if bumpRevision {
		next.revision = e.state.revision + 1
	}

	// Phase 4 (D-81/D-84): the config half of the write set, derived by the same
	// scratch-vs-committed diff the item rows use, so a mutation cannot forget to
	// persist what it changed. A nil result leaves game.config_json untouched.
	configJSON, err := next.configJSON(e.state)
	if err != nil {
		return err // nothing persisted, nothing published
	}

	// Phase 4 (D-78): the reset half of the write set. Every field above is
	// growth-only — a cleared trail is invisible to bidTail, sealed bids have no
	// diff at all, and a nil Runtime means "unchanged" rather than "removed" — so
	// the removals travel as the explicit write set the store needs, read from
	// the intent Reset recorded rather than inferred from a difference.
	isReset := next.reset != ""

	if err := e.store.Commit(store.Mutation{
		Phase:         string(next.phase),
		CurrentItemID: next.currentItemID,
		Revision:      next.revision,
		UpdatedAt:     e.now().UnixMilli(),
		ConfigJSON:    configJSON,
		Players:       next.playerRows(),
		// Phase 3 (H3): the auction write set is derived by diffing the scratch
		// state against the committed one, so a mutation cannot forget to
		// persist what it changed — the single most important property of the
		// crash drill (RT-06).
		Items:        next.changedItems(e.state),
		DeletedItems: next.deletedItems(e.state),
		Runtime:      next.runtimeRow(e.state),
		Bids:         next.bidTail(e.state),
		// A reset empties the trail, the sealed bids and the runtime rows;
		// `DeletedPlayers` is the one removal a diff CAN see, because a player
		// row does have an in-memory identity to compare against.
		ClearBids:       isReset,
		ClearSealedBids: isReset,
		DeleteRuntime:   isReset,
		DeletedPlayers:  next.deletedPlayers(e.state),
	}); err != nil {
		if store.IsUniqueViolation(err) {
			// A UNIQUE collision is a rejection, not corruption: the write set
			// was refused, the live state was never touched, and the caller
			// answers with a typed error. Everything else failures hard.
			return err
		}
		e.fatal(fmt.Errorf("commit revision %d: %w", next.revision, err))
		return err
	}

	e.state = next // memory never runs ahead of disk
	e.revision.Store(next.revision)

	if emit != nil {
		emit(view{e})
	}
	e.publishLocked()
	return nil
}

// Join resolves one client's join (D-23/D-30) under the exclusive lock, so the
// registration, hello and snapshot cannot be split by a concurrent mutation.
// emit, when non-nil, receives hello first and the role projection second.
func (e *Engine) Join(req JoinRequest, emit func(Hello, any)) (JoinResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if req.Role != RolePlayer {
		// Screen and admin carry no identity and mutate nothing (D-23); they
		// only need the current role projection, built and handed over inside
		// this same critical section so the handshake pair is atomic for every
		// role. Admin is authorized by the hub BEFORE this call (D-76/ADMN-04),
		// so no key is ever needed — or accepted — here.
		if req.Role != RoleScreen && req.Role != RoleAdmin {
			return JoinResult{}, fmt.Errorf("game: join role %q is not one of player|screen|admin", req.Role)
		}
		res := JoinResult{Role: req.Role, Rev: e.state.revision}
		if emit != nil {
			emit(Hello{Role: req.Role, Rev: e.state.revision}, e.snapshotLocked(req.Role, ""))
		}
		return res, nil
	}

	// D-30's ladder, in order: a valid token resumes; an unknown token with no
	// nickname is rejected; anything else is a fresh join with the nickname.
	if p := e.state.playerByToken(req.Token); p != nil {
		return e.resumeLocked(p, emit)
	}
	if req.Token != "" && req.Nickname == "" {
		return JoinResult{}, &JoinError{
			Code:    CodeTokenInvalid,
			Message: "this token is unknown; join with a nickname to start a new identity",
		}
	}
	return e.newPlayerLocked(req, emit)
}

// resumeLocked is D-30 (1): the token wins, the nickname on the wire is ignored,
// and the only thing that changes is presence — last_seen_at and the connected
// flag, which ride the current revision because presence is not a game mutation
// (D-37).
func (e *Engine) resumeLocked(p *player, emit func(Hello, any)) (JoinResult, error) {
	id := p.id
	now := e.now().UnixMilli()

	err := e.applyLocked(func(s *state) error {
		mine := s.players[id]
		mine.lastSeenAt = now
		s.connected[id] = true
		return nil
	}, false, func(v View) {
		if emit != nil {
			emit(Hello{
				Role:     RolePlayer,
				PlayerID: id,
				Nickname: p.nickname,
				Token:    p.token,
				Rev:      v.Rev(),
			}, v.Snapshot(RolePlayer, id))
		}
	})
	if err != nil {
		return JoinResult{}, err
	}
	return JoinResult{Role: RolePlayer, PlayerID: id, Token: p.token, Resumed: true, Rev: e.state.revision}, nil
}

// newPlayerLocked is D-30 (2)/(4): mint a fresh identity. Creating a player is a
// game mutation — the lobby is observable state — so it bumps the revision and
// persists the new row in the same transaction (D-35).
func (e *Engine) newPlayerLocked(req JoinRequest, emit func(Hello, any)) (JoinResult, error) {
	normalized, ok := NormalizeNickname(req.Nickname)
	if !ok {
		return JoinResult{}, &JoinError{
			Code:    CodeNicknameInvalid,
			Message: fmt.Sprintf("nickname must be 1..%d characters after trimming", MaxNicknameRunes),
		}
	}
	key := NicknameKey(normalized)
	if existing := e.state.playerByNicknameKey(key); existing != nil {
		// D-32: a nickname is reserved by its row. A connected owner cannot
		// prove ownership (no auth by design), and a disconnected one keeps the
		// name — so both cases are the same rejection. D-80 adds exactly one
		// exception: a row the presenter RELEASED is claimable by the next device
		// that types its nickname, because its token can never resume it again.
		if !isReleasedToken(existing.token) {
			return JoinResult{}, &JoinError{Code: CodeNicknameTaken, Message: "that nickname is already taken"}
		}
		return e.claimReleasedLocked(existing, emit)
	}

	id, err := newPlayerID()
	if err != nil {
		return JoinResult{}, err
	}
	token, err := NewToken()
	if err != nil {
		return JoinResult{}, err
	}

	now := e.now().UnixMilli()
	fresh := &player{
		id:          id,
		nickname:    normalized,
		nicknameKey: key,
		token:       token,
		budget:      e.state.config.StartingBudget,
		joinedAt:    now,
		lastSeenAt:  now,
	}

	err = e.applyLocked(func(s *state) error {
		s.players[id] = fresh
		s.order = append(s.order, id)
		s.connected[id] = true
		return nil
	}, true, func(v View) {
		if emit != nil {
			emit(Hello{
				Role:     RolePlayer,
				PlayerID: id,
				Nickname: normalized,
				Token:    token,
				Rev:      v.Rev(),
			}, v.Snapshot(RolePlayer, id))
		}
	})
	if err != nil {
		// The in-memory check above decides in the normal case; the UNIQUE index
		// is the atomic backstop (D-28), and its violation is the same
		// rejection.
		if store.IsUniqueViolation(err) {
			return JoinResult{}, &JoinError{Code: CodeNicknameTaken, Message: "that nickname is already taken"}
		}
		return JoinResult{}, err
	}
	return JoinResult{Role: RolePlayer, PlayerID: id, Token: token, Rev: e.state.revision}, nil
}

// claimReleasedLocked is D-80's reclaim arm: the device that types a released
// row's nickname takes that row over. The identity is the SAME player — id, the
// display nickname, budget, total value and joined_at all survive, which is what
// makes the release cheap insurance rather than a quiet deletion of someone's
// economics — and the token is a fresh mint, because the device that used to hold
// it must not be able to resume a session the presenter deliberately ended.
//
// Rewriting the row is a durable mutation, so it bumps the revision and emits the
// fresh identity's hello plus its own role snapshot inside the same critical
// section (D-23/D-35). The claim deliberately reports Resumed == false: this is a
// registration, not a token resume, and the device stores the new token exactly
// as it would after any other join.
func (e *Engine) claimReleasedLocked(p *player, emit func(Hello, any)) (JoinResult, error) {
	id := p.id
	nickname := p.nickname

	token, err := NewToken()
	if err != nil {
		return JoinResult{}, err
	}
	now := e.now().UnixMilli()

	err = e.applyLocked(func(s *state) error {
		mine := s.players[id]
		mine.token = token
		mine.lastSeenAt = now
		s.connected[id] = true
		return nil
	}, true, func(v View) {
		if emit != nil {
			emit(Hello{
				Role:     RolePlayer,
				PlayerID: id,
				Nickname: nickname,
				Token:    token,
				Rev:      v.Rev(),
			}, v.Snapshot(RolePlayer, id))
		}
	})
	if err != nil {
		return JoinResult{}, err
	}
	return JoinResult{Role: RolePlayer, PlayerID: id, Token: token, Rev: e.state.revision}, nil
}

// Connect marks a player connected and announces it (D-37). It is a no-op for
// an unknown id and for a player already in that state.
func (e *Engine) Connect(playerID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setPresenceLocked(playerID, true)
}

// Disconnect marks a player disconnected, unless this connection is not the one
// that owns the player (D-31: the caller is responsible for the
// compare-and-remove decision; this method only applies the consequence).
func (e *Engine) Disconnect(playerID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.setPresenceLocked(playerID, false)
}

// setPresenceLocked flips one player's connected flag (D-37). Presence is
// in-memory only and never bumps the revision; last_seen_at is persisted in the
// same write-through transaction as everything else, at the current revision.
func (e *Engine) setPresenceLocked(playerID string, connected bool) error {
	if e.state.players[playerID] == nil {
		// Unknown id: a no-op, so a teardown can never depend on the row still
		// existing (a Phase 4 game.reset_players deletes players).
		return nil
	}
	if e.state.connected[playerID] == connected {
		return nil // already in that state: no write, no fan-out
	}

	now := e.now().UnixMilli()
	return e.applyLocked(func(s *state) error {
		s.connected[playerID] = connected
		s.players[playerID].lastSeenAt = now
		return nil
	}, false, nil)
}

// Section runs fn with the exclusive lock held. It is the hub's way to build a
// resync snapshot and enqueue it with no producer able to interleave (D-42).
//
// fn must not block and must not call back into the engine: the lock is already
// held, and enqueueing into a buffered channel is the intended (and only)
// operation.
func (e *Engine) Section(fn func(v View) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return fn(view{e})
}

// Snapshot builds one client's role projection under a read lock, for callers
// that are not already inside a Section.
func (e *Engine) Snapshot(role Role, playerID string) any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.snapshotLocked(role, playerID)
}

// snapshotLocked builds the role projection. The caller must hold the lock:
// project.go reads the unexported state directly, which is the whole point of
// the type boundary (D-26).
func (e *Engine) snapshotLocked(role Role, playerID string) any {
	now := e.now()
	switch role {
	case RolePlayer:
		return e.state.playerView(now, playerID)
	case RoleScreen:
		return e.state.screenView(now)
	case RoleAdmin:
		return e.state.adminView(now)
	default:
		return nil
	}
}

// publishLocked hands the committed revision and a lock-held View to the
// fan-out edge, if one is installed, and does so BEFORE the lock is released —
// the enqueue-in-lock invariant (PLAN.md §3.1 #5 / D-33).
func (e *Engine) publishLocked() {
	p := e.publisher.Load()
	if p == nil {
		return
	}
	(*p)(e.state.revision, view{e}, e.state.events)
}

// view is the read-only window the engine hands to Publisher and Section
// callbacks. It is only ever constructed with the lock held.
type view struct{ e *Engine }

func (v view) Rev() int64 { return v.e.state.revision }

func (v view) Snapshot(role Role, playerID string) any {
	return v.e.snapshotLocked(role, playerID)
}

// Tick is the D-56 timing frame. It is built from the same lock-held state a
// mutation just committed, with the engine's injected clock (D-60), so
// remaining_ms is server-computed and can never disagree with the deadline the
// settle path will honour.
func (v view) Tick() (PriceTick, bool) { return v.e.state.priceTick(v.e.now()) }

// clone deep-copies the state so a mutation that fails validation can never
// leave the live state half-updated, and so the committed state is published
// atomically as one pointer swap.
//
// events and reset are deliberately NOT copied: each mutation starts with an
// empty event list and no reset intent, so the list on the published state is
// exactly what that mutation produced (D-57) and a later mutation can never
// inherit a clear it did not ask for.
func (s *state) clone() *state {
	next := &state{
		phase:         s.phase,
		revision:      s.revision,
		currentItemID: s.currentItemID,
		config:        s.config,
		players:       make(map[string]*player, len(s.players)),
		order:         append([]string(nil), s.order...),
		connected:     make(map[string]bool, len(s.connected)),
		items:         make(map[string]*item, len(s.items)),
		itemOrder:     append([]string(nil), s.itemOrder...),
		sealedBids:    append([]sealedBid(nil), s.sealedBids...),
		bids:          append([]bid(nil), s.bids...),
		// D-83: the candidate list is immutable boot data, so the slice header is
		// copied and the strings are shared — unlike `bids`/`sealedBids`, nothing
		// ever appends to it.
		joinURLCandidates: s.joinURLCandidates,
	}
	if s.currentAuction != nil {
		run := *s.currentAuction
		next.currentAuction = &run
	}
	for id, p := range s.players {
		copied := *p
		next.players[id] = &copied
	}
	for id, connected := range s.connected {
		next.connected[id] = connected
	}
	for id, it := range s.items {
		copied := *it
		next.items[id] = &copied
	}
	return next
}

// playerRows projects the in-memory players back into their durable rows.
func (s *state) playerRows() []store.PlayerRow {
	rows := make([]store.PlayerRow, 0, len(s.players))
	for _, id := range s.order {
		p := s.players[id]
		rows = append(rows, store.PlayerRow{
			ID:          p.id,
			Nickname:    p.nickname,
			NicknameKey: p.nicknameKey,
			Token:       p.token,
			Budget:      p.budget,
			TotalValue:  p.totalValue,
			JoinedAt:    p.joinedAt,
			LastSeenAt:  p.lastSeenAt,
		})
	}
	return rows
}

// itemRow projects one in-memory item into its durable row. Every nullable
// column is normalized 0 → SQL NULL, which is the same direction the table uses
// on the way back (LoadItems reads NULL as 0).
func itemRow(it *item) store.ItemRow {
	opt := func(v int64) *int64 {
		if v == 0 {
			return nil
		}
		return &v
	}
	return store.ItemRow{
		ID: it.id, OrderIndex: it.orderIndex, Name: it.name, Description: it.description,
		ImageURL: it.imageURL, AuctionType: it.auctionType, TruthValue: it.truthValue,
		HintMin: opt(it.hintMin), HintMax: opt(it.hintMax), StartPrice: it.startPrice,
		MinIncrement: opt(it.minIncrement), FloorPrice: opt(it.floorPrice),
		DecrementStep: opt(it.decrementStep), DecrementIntervalMS: opt(it.decrementIntervalMS),
		DurationMS: opt(it.durationMS), AntiSnipeMode: it.antiSnipeMode,
		AntiSnipeWindowMS: opt(it.antiSnipeWindowMS), MaxExtensions: opt(it.maxExtensions),
		Status: it.status,
	}
}

// itemFromRow is itemRow's inverse: a persisted row becomes the in-memory value
// struct with every SQL NULL normalized to 0, which is the same convention
// itemRow writes back as SQL NULL. Boot is the only caller (D-62).
func itemFromRow(row store.ItemRow) *item {
	deref := func(v *int64) int64 {
		if v == nil {
			return 0
		}
		return *v
	}
	return &item{
		id: row.ID, orderIndex: row.OrderIndex, name: row.Name, description: row.Description,
		imageURL: row.ImageURL, auctionType: row.AuctionType, truthValue: row.TruthValue,
		hintMin: deref(row.HintMin), hintMax: deref(row.HintMax), startPrice: row.StartPrice,
		minIncrement: deref(row.MinIncrement), floorPrice: deref(row.FloorPrice),
		decrementStep: deref(row.DecrementStep), decrementIntervalMS: deref(row.DecrementIntervalMS),
		durationMS: deref(row.DurationMS), antiSnipeMode: row.AntiSnipeMode,
		antiSnipeWindowMS: deref(row.AntiSnipeWindowMS), maxExtensions: deref(row.MaxExtensions),
		status: row.Status,
	}
}

// changedItems is H3's diff: only the rows whose value differs from the
// committed state are written, in presentation order. `item` is a flat
// comparable struct, so `*a != *b` is the entire "this row changed" test; if a
// later phase adds a slice or map field, this must become an explicit predicate
// instead.
func (s *state) changedItems(prev *state) []store.ItemRow {
	var rows []store.ItemRow
	for _, id := range s.itemOrder {
		it := s.items[id]
		if old, existed := prev.items[id]; existed && *old == *it {
			continue
		}
		rows = append(rows, itemRow(it))
	}
	return rows
}

// configJSON is D-81/D-84's config write set: the serialized configuration when
// the scratch state's copy differs from the committed one, and nil when it did
// not move (the store then leaves game.config_json exactly as it was).
//
// A diff rather than a recorded intent, for the same reason changedItems is one:
// `engineConfig` is a flat comparable struct, so the comparison cannot be
// forgotten, and a reset that leaves the configuration alone can never resurrect
// a stale copy. The marshal can only fail on a programming error, and it fails
// BEFORE anything is written, so a bad write set is never committed.
func (s *state) configJSON(prev *state) (*string, error) {
	if s.config == prev.config {
		return nil, nil
	}
	raw, err := json.Marshal(s.config)
	if err != nil {
		return nil, fmt.Errorf("game: marshalling the config write set %+v: %w", s.config, err)
	}
	encoded := string(raw)
	return &encoded, nil
}

// deletedItems names the ids the committed state held and the scratch copy does
// not: the write set must say so explicitly, because a map diff cannot see a
// removal without a key walk.
func (s *state) deletedItems(prev *state) []string {
	var ids []string
	for _, id := range prev.itemOrder {
		if _, ok := s.items[id]; !ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// deletedPlayers is deletedItems for the D-78 `clear_players` arm: the ids the
// committed state held and the scratch copy does not. The store runs this delete
// after the sealed-bid and bid clears, because both tables reference players.
func (s *state) deletedPlayers(prev *state) []string {
	var ids []string
	for _, id := range prev.order {
		if _, ok := s.players[id]; !ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// runtimeRow is the persisted timing of the current run, or nil when the timing
// did not move (D-54: the row changes on a start, an extend or a settle — not on
// a bid that left the deadline alone). The revision is the mutation's revision:
// it records when the timing last moved.
func (s *state) runtimeRow(prev *state) *store.RuntimeRow {
	run := s.currentAuction
	if run == nil {
		return nil
	}
	if prev.currentAuction != nil && sameRuntime(prev.currentAuction, run) {
		return nil
	}
	row := &store.RuntimeRow{
		ItemID:         run.itemID,
		StartedAt:      run.startedAt,
		Revision:       s.revision,
		ExtensionsUsed: run.extensionsUsed,
	}
	if run.deadlineAt > 0 {
		deadline := run.deadlineAt
		row.DeadlineAt = &deadline
	}
	if run.settledAt > 0 {
		settled := run.settledAt
		row.SettledAt = &settled
	}
	return row
}

// sameRuntime compares the persisted halves of two runs. The settle reveal is
// deliberately excluded: it is derivable from the trail and the item, and a
// second persisted copy of the economics would be a second source of truth.
func sameRuntime(a, b *auctionRun) bool {
	return a.itemID == b.itemID &&
		a.startedAt == b.startedAt &&
		a.deadlineAt == b.deadlineAt &&
		a.extensionsUsed == b.extensionsUsed &&
		a.settledAt == b.settledAt
}

// bidTail is the append-only tail: the bids committed since prev (H4). The trail
// is never truncated, so the slice suffix is exactly the new rows — and each row
// travels with ITS OWN kind, so an acceptance is persisted as `accept` and a
// ladder bid as `bid` (D-98) without this function knowing which flow wrote it.
func (s *state) bidTail(prev *state) []store.BidRow {
	if len(s.bids) <= len(prev.bids) {
		return nil
	}
	rows := make([]store.BidRow, 0, len(s.bids)-len(prev.bids))
	for _, b := range s.bids[len(prev.bids):] {
		rows = append(rows, store.BidRow{
			ItemID: b.itemID, PlayerID: b.playerID,
			Amount: b.amount, Kind: b.kind, CreatedAt: b.createdAt,
		})
	}
	return rows
}

// playerByToken resolves D-30 (1). The empty token never matches: "no token" is
// a fresh join, not a resume of a player whose token is empty. A released row is
// refused as well (D-80): the value it now holds is the sentinel, and a client
// that echoed the sentinel back must not be able to resume the identity it used
// to protect.
func (s *state) playerByToken(token string) *player {
	if token == "" || isReleasedToken(token) {
		return nil
	}
	for _, id := range s.order {
		if p := s.players[id]; p.token == token {
			return p
		}
	}
	return nil
}

// playerByNicknameKey resolves D-28's uniqueness key in memory. It is the fast
// path; the UNIQUE index in SQLite is the atomic guarantee.
func (s *state) playerByNicknameKey(key string) *player {
	for _, id := range s.order {
		if p := s.players[id]; p.nicknameKey == key {
			return p
		}
	}
	return nil
}
