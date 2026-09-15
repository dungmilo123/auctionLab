package game

import (
	"fmt"
	"log"
)

// This file is the boot half of RT-05/D-62: an engine never starts from nothing,
// every start is a RESUME. `New` calls loadAuctionState once the game row and
// the players are in memory, and settleDueAtBoot once the state is installed, so
// a deadline that passed while the process was down settles before cmd/server
// builds the mux — no listener, no hub and no publisher exist yet, which is what
// makes "no fan-out at boot" structural rather than a special case.
//
// The one rule this file enforces: durable state is trusted only after the
// phase↔item↔runtime triple checks out. A LIVE phase that cannot be
// reconstructed fails hard instead of inventing a deadline, a top bid or an
// extension count (T-03-04).

// loadAuctionState rebuilds the durable auction half of the state (D-49/D-62):
// the item rows in presentation order, the append-only bid trail, and then the
// validated current run. Every load error is returned to main, which exits
// non-zero before ListenAndServe — a silently empty lobby that disagrees with
// the database is exactly the failure D-39/D-62 forbid.
func (e *Engine) loadAuctionState() error {
	s := e.state

	rows, err := e.store.LoadItems()
	if err != nil {
		return err
	}
	for _, row := range rows {
		it := itemFromRow(row)
		s.items[it.id] = it
		s.itemOrder = append(s.itemOrder, it.id)
	}

	bids, err := e.store.LoadBids()
	if err != nil {
		return err
	}
	for _, row := range bids {
		// Only accepted bids join the trail. The table is the append-only audit
		// and a later phase may write other kinds; the same filter the boot
		// reconstruction query uses keeps a foreign row out of the ladder (H5).
		if row.Kind != "bid" {
			continue
		}
		s.bids = append(s.bids, bid{
			itemID:    row.ItemID,
			playerID:  row.PlayerID,
			amount:    row.Amount,
			createdAt: row.CreatedAt,
		})
	}

	return e.validateResumedAuction()
}

// validateResumedAuction is the phase↔item↔runtime matrix (D-62). It runs on
// every boot, including the ones that never went LIVE, because the invariant it
// checks is what makes the next mutation legal:
//
//   - LOBBY and FINALE have no current item;
//   - ITEM_INTRO names an existing item whose row is still `intro`;
//   - LIVE names an existing `live` item with a runtime row (and, for an
//     English item, a persisted deadline);
//   - SETTLED names an existing `settled` item whose runtime row proves the
//     settle with a `settled_at`.
//
// A violation returns an error naming the offending value rather than a
// repaired state: repairing would be guessing at what the room saw.
func (e *Engine) validateResumedAuction() error {
	s := e.state
	current := s.items[s.currentItemID]

	switch s.phase {
	case PhaseLobby, PhaseFinale:
		if s.currentItemID != "" {
			return fmt.Errorf("game: phase %s still names current item %q, but a %s state has no current item", s.phase, s.currentItemID, s.phase)
		}
		s.currentAuction = nil
		return nil

	case PhaseItemIntro:
		if current == nil {
			return fmt.Errorf("game: phase ITEM_INTRO names current item %q that is not in the items table", s.currentItemID)
		}
		if current.status != "intro" {
			return fmt.Errorf("game: phase ITEM_INTRO but item %q is %s, want intro", current.id, current.status)
		}
		s.currentAuction = nil
		return nil
	}

	// LIVE and SETTLED both describe a run that exists on disk.
	if current == nil {
		return fmt.Errorf("game: phase %s names current item %q that is not in the items table", s.phase, s.currentItemID)
	}
	rt, ok, err := e.store.LoadRuntime(current.id)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("game: phase %s but item %q has no auction_runtime row — refusing to invent an auction (T-03-04)", s.phase, current.id)
	}

	run := &auctionRun{
		itemID:         current.id,
		startedAt:      rt.StartedAt,
		extensionsUsed: rt.ExtensionsUsed,
	}
	if rt.DeadlineAt != nil {
		run.deadlineAt = *rt.DeadlineAt
	}
	if rt.SettledAt != nil {
		run.settledAt = *rt.SettledAt
	}

	if s.phase == PhaseLive {
		if current.status != "live" {
			return fmt.Errorf("game: phase LIVE but item %q is %s, want live", current.id, current.status)
		}
		if current.auctionType == "english" && rt.DeadlineAt == nil {
			// A resumed English auction whose wall clock is unknown could
			// neither settle nor tick; there is no correct value to guess.
			return fmt.Errorf("game: phase LIVE on english item %q with no persisted deadline — a resumed English auction must have one (D-62)", current.id)
		}
		s.currentAuction = run
		return nil
	}

	// SETTLED: the runtime row is the proof the settle happened, and the reveal
	// itself is rebuilt by the projection (project.go) because the row persists
	// timing only — a late joiner must still be able to render it (D-62/D-67).
	if current.status != "settled" {
		return fmt.Errorf("game: phase SETTLED but item %q is %s, want settled", current.id, current.status)
	}
	if rt.SettledAt == nil {
		return fmt.Errorf("game: phase SETTLED but item %q has no settled_at — the settle was never persisted", current.id)
	}
	s.currentAuction = run
	return nil
}

// settleDueAtBoot is D-55's third trigger: a LIVE run whose wall-clock deadline
// has already passed settles NOW, through the same settleLocked mutation the
// ticker and the force-end use, so the budget moves exactly once no matter which
// trigger gets there first. No publisher is installed at this point, so the
// missing fan-out is the ordinary path, not a suppressed special case.
func (e *Engine) settleDueAtBoot() error {
	s := e.state
	run := s.currentAuction
	if s.phase != PhaseLive || run == nil {
		return nil // no run, no timing action
	}

	itemID, deadline := run.itemID, run.deadlineAt
	now := e.now().UnixMilli()
	if deadline == 0 || now < deadline {
		log.Printf("game: resumed LIVE item=%s deadline=%d", itemID, deadline)
		return nil
	}

	if err := e.applyLocked(func(s *state) error { return s.settleLocked(now) }, true, nil); err != nil {
		return err
	}
	log.Printf("game: resumed SETTLED item=%s deadline=%d (deadline passed while the process was down; settled at boot, revision %d)", itemID, deadline, e.revision.Load())
	return nil
}

// ResumedAuction reports the run this process picked up from disk, for the boot
// log (D-62: an operator must be able to read the resumed item and deadline
// without opening the database). Both values are zero when no run is live.
func (e *Engine) ResumedAuction() (itemID string, deadlineAt int64) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if run := e.state.currentAuction; run != nil {
		return run.itemID, run.deadlineAt
	}
	return "", 0
}
