package game

import (
	"errors"
	"fmt"
)

// This file is ADMN-03's reset contract (D-78): `admin.game.reset {mode}` is an
// ordinary applyLocked mutation that returns a played game to LOBBY. The
// interesting half is the write set — the reset is the one mutation that REMOVES
// rows, and Phase 3's write-set helpers are all growth-only, so the removals
// travel as explicit store.Mutation fields instead of being inferred from a diff
// (see engine.go's applyLocked and research Pitfall 5).
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
// vocabulary is the validator (T-04-03). A LOBBY reset with nothing to clear is
// a no-op: it changes neither the revision nor the persisted state.
func (e *Engine) Reset(mode ResetMode) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	switch mode {
	case ResetKeepPlayers, ResetClearPlayers:
	default:
		return fmt.Errorf("game: reset mode %q is not one of keep_players|clear_players", mode)
	}

	// RED-phase stub: the write set is not wired yet, so nothing is persisted and
	// the caller learns the reset did not happen rather than reading a status
	// that silently disagrees with the database.
	return errors.New("game: reset is not implemented yet")
}
