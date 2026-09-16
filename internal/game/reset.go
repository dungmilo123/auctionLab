package game

import "fmt"

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
	// player row as well; the hub answers that by closing the players' sockets,
	// so their devices land back on the join form.
	ResetClearPlayers ResetMode = "clear_players"
)

// Reset is `admin.game.reset {mode}` (D-78): ONE applyLocked mutation that
// removes the bid trail, the sealed bids and the runtime rows, restores every
// budget, puts every item back to `pending`, clears the current item and returns
// the phase to LOBBY — persisting the whole write set in the same transaction as
// the revision bump (D-34) and fanning out `phase.changed` followed by the role
// snapshots inside the engine lock (D-57).
//
// An unknown mode is refused before anything is touched, so the closed
// vocabulary is the validator (T-04-03).
func (e *Engine) Reset(mode ResetMode) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch mode {
	case ResetKeepPlayers:
	default:
		return fmt.Errorf("game: reset mode %q is not one of keep_players|clear_players", mode)
	}

	return e.applyLocked(func(s *state) error {
		return s.resetLocked(mode)
	}, true, nil)
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

	// Every identity starts the new round on the configured budget. The player
	// row itself — id, nickname, token, joined_at — is untouched.
	for _, id := range s.order {
		p := s.players[id]
		p.budget = s.config.StartingBudget
		p.totalValue = 0
	}

	s.phase = PhaseLobby
	s.reset = mode
	s.events = append(s.events,
		Event{Type: EventPhaseChanged, Data: s.phaseChanged(PhaseLobby)},
		Event{Type: EventSnapshot},
	)
	return nil
}
