package game

import (
	"fmt"
	"strings"
)

// This file is ADMN-03's reset contract (D-78): `admin.game.reset {mode}` is an
// ordinary applyLocked mutation that returns a played game to LOBBY. The
// interesting half is the write set — the reset is the one mutation that REMOVES
// rows, and Phase 3's write-set helpers are all growth-only, so the removals
// travel as explicit store.Mutation fields (see applyLocked) and the intent is
// recorded on the scratch state instead of being inferred from a difference.
//
// D-80's ReleaseSession lives here too: releasing a stranded session and letting
// the same nickname claim its row back is the same "return a played identity to a
// usable state" concern, and it must run on the same one mutation path.

// ResetMode is D-78's vocabulary. The literals are PLAN.md §6's, so they are part
// of the frozen wire catalog (D-24) and are never renamed.
type ResetMode string

const (
	// ResetKeepPlayers returns the game to LOBBY and keeps every identity: the
	// item configuration survives, and every player's budget is restored to
	// `config.starting_budget` with `total_value` back to 0.
	ResetKeepPlayers ResetMode = "keep_players"
	// ResetClearPlayers does everything keep_players does and deletes every
	// player row as well; the hub answers that by closing the sockets of exactly
	// the identities this commit deleted, so their devices land back on the join
	// form while a phone that joined after the commit keeps its socket.
	ResetClearPlayers ResetMode = "clear_players"
)

// releasedTokenPrefix marks a player row whose session the presenter released
// (D-80). NewToken mints 64 lowercase hex characters, so ':' can never appear in
// a live token: the sentinel is impossible to mint, and — because it carries the
// player id — two released rows hold two DIFFERENT values in a column that is
// `NOT NULL UNIQUE`. An empty string could not: the second release would collide
// with the first and be refused (research Pitfall 7).
//
// Reversibility: costly. This value is persisted in players.token, so changing
// the sentinel later means recognising the old form during a migration — it is an
// internal persistence contract, not a constant to refactor freely.
const releasedTokenPrefix = "released:"

// releasedToken is the value a released row's `token` column holds.
func releasedToken(playerID string) string { return releasedTokenPrefix + playerID }

// isReleasedToken reports whether a persisted (or submitted) token is the D-80
// sentinel. It is the ONE test the three carrier sites share: the token lookup
// skips released rows, the resume arm answers token_invalid, and the nickname arm
// treats the row as claimable.
func isReleasedToken(token string) bool { return strings.HasPrefix(token, releasedTokenPrefix) }

// ResetResult reports what one `admin.game.reset` did.
type ResetResult struct {
	// Changed reports whether the command committed. A keep_players reset of an
	// already-fresh lobby is D-78's no-op: nothing is persisted, nothing is
	// fanned out, and the transport edge answers the presenter's own connection
	// instead — the panel's in-flight flag is derived from a snapshot.
	Changed bool
	// Cleared lists the player ids the commit deleted (clear_players only), in
	// the order the committed state held them. The hub ends exactly these
	// sockets: it reads a registry snapshot, so "every connection that is a
	// player by now" would also close a phone that joined while the presenter
	// was clicking Reset — a new row this commit never touched.
	Cleared []string
}

// Reset is `admin.game.reset {mode}` (D-78): ONE applyLocked mutation that
// removes the bid trail, the sealed bids and the runtime rows, restores every
// budget, puts every item back to `pending`, clears the current item and returns
// the phase to LOBBY — persisting the whole write set in the same transaction as
// the revision bump (D-34) and fanning out `phase.changed` followed by the role
// snapshots inside the engine lock (D-57).
//
// An unknown mode is refused before anything is touched, so the closed
// vocabulary is the validator (T-04-03).
//
// The result carries the two facts the transport edge cannot derive from the
// frame stream: whether the command wrote at all, and — when it did — exactly
// which identities it deleted.
func (e *Engine) Reset(mode ResetMode) (ResetResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch mode {
	case ResetKeepPlayers, ResetClearPlayers:
	default:
		return ResetResult{}, fmt.Errorf("game: reset mode %q is not one of keep_players|clear_players", mode)
	}

	// D-78: keep_players over an already-fresh lobby repairs nothing, so it
	// writes nothing. The test is on what a reset would CHANGE rather than on the
	// phase alone, so a reset with anything left to do always runs; clear_players
	// is deliberately exempt, because a lobby full of phones has a room to clear
	// even though no item has played.
	if mode == ResetKeepPlayers && e.state.resetIsNoOp() {
		return ResetResult{}, nil
	}

	var result ResetResult
	if err := e.applyLocked(func(s *state) error {
		if err := s.resetLocked(mode); err != nil {
			return err
		}
		// The ids the store is about to delete, read from the same
		// scratch-vs-committed diff applyLocked uses for the DeletedPlayers
		// write set, so the sockets the hub ends are exactly the rows this
		// commit removed — not "whoever is a player when the loop runs".
		result.Cleared = s.deletedPlayers(e.state)
		return nil
	}, true, nil); err != nil {
		return ResetResult{}, err
	}
	result.Changed = true
	return result, nil
}

// resetIsNoOp reports whether the committed state is already exactly what a
// keep_players reset produces: LOBBY, nothing to clear, every item pending and
// every identity on the starting budget.
func (s *state) resetIsNoOp() bool {
	if s.phase != PhaseLobby || len(s.bids) > 0 || len(s.sealedBids) > 0 || s.currentAuction != nil {
		return false
	}
	for _, id := range s.itemOrder {
		if s.items[id].status != "pending" {
			return false
		}
	}
	for _, id := range s.order {
		p := s.players[id]
		if p.budget != s.config.StartingBudget || p.totalValue != 0 {
			return false
		}
	}
	return true
}

// resetLocked rewrites the scratch state into a fresh lobby. It runs inside
// applyLocked, so nothing here is durable until the write set it produces
// commits.
//
// The scratch state's `reset` field is what makes the removals durable: the
// store cannot see a cleared trail, a sealed-bid row it never loaded, or the
// difference between "the run ended" and "the runtime rows must go", so the
// intent is recorded here and applyLocked translates it into the explicit
// store.Mutation fields.
func (s *state) resetLocked(mode ResetMode) error {
	if s.phase != PhaseLobby && !CanTransition(s.phase, PhaseLobby) {
		// Every row of the §6 machine lists LOBBY, so this is a guard against a
		// future edit rather than a reachable refusal.
		return fmt.Errorf("game: the machine does not declare %s → LOBBY", s.phase)
	}

	s.bids = nil
	s.sealedBids = nil
	s.currentAuction = nil
	s.currentItemID = ""

	// D-78 keeps the item configuration and rewrites only the status: a reset
	// round must be playable again, and the rows are the presenter's own setup.
	for _, id := range s.itemOrder {
		s.items[id].status = "pending"
	}

	if mode == ResetClearPlayers {
		// The room goes with the timeline. The store deletes the player rows
		// only after it has emptied bids and sealed_bids, because both reference
		// them (T-04-05); in memory the presence map goes too, so no ghost
		// connection can keep a deleted identity "connected".
		s.players = make(map[string]*player)
		s.order = nil
		s.connected = make(map[string]bool)
	} else {
		// Every identity starts the new round on the configured budget. The row
		// itself — id, nickname, token, joined_at — is untouched.
		for _, id := range s.order {
			p := s.players[id]
			p.budget = s.config.StartingBudget
			p.totalValue = 0
		}
	}

	s.phase = PhaseLobby
	s.reset = mode
	s.events = append(s.events,
		Event{Type: EventPhaseChanged, Data: s.phaseChanged(PhaseLobby)},
		Event{Type: EventSnapshot},
	)
	return nil
}

// ReleaseSession is `admin.player.release {id}` (D-80): the presenter's insurance
// against a device that can no longer be reached — a phone that changed networks,
// a browser that lost its storage, an origin-scoped token after the tunnel URL
// rotated. Releasing writes the row's token to the sentinel above, so the token
// that device still holds can never resume the identity again (D-30 (1) is gone
// for that row) and the row's nickname becomes claimable by the next device that
// types it.
//
// The write is durable, so it bumps the revision and ends with the role snapshot
// (D-35): the admin's Players tab learns about it from the same fan-out as
// everything else, and the released player's own socket is the hub's to close.
// Releasing an already-released row is a no-op, and — like the reset's own
// no-op — it reports that with a false so the transport edge can answer the
// presenter's connection, whose in-flight flag waits for a frame.
func (e *Engine) ReleaseSession(playerID string) (changed bool, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	p := e.state.players[playerID]
	if p == nil {
		return false, fmt.Errorf("game: player.release %q does not exist", playerID)
	}
	if isReleasedToken(p.token) {
		return false, nil // already released: nothing left to write, no revision move
	}

	if err := e.applyLocked(func(s *state) error {
		s.players[playerID].token = releasedToken(playerID)
		// Presence is in-memory and rides the current revision, but a released
		// identity must not read as "connected" to the admin while the hub is
		// still tearing its socket down.
		s.connected[playerID] = false
		s.events = append(s.events, Event{Type: EventSnapshot})
		return nil
	}, true, nil); err != nil {
		return false, err
	}
	return true, nil
}
