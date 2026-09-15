package game

import "slices"

// This file is the PLAN.md §6 state machine. Phase 2 wires the machine only as
// far as LOBBY — nothing in this plan moves the phase — and that is exactly why
// the edges are declared now: Phase 3's item flow must not be able to invent a
// transition later.

// Phase is one node of PLAN.md §6's state machine. The string values are the
// wire values (a snapshot carries `phase`), so they are part of the protocol's
// frozen vocabulary (D-24).
type Phase string

// The five phases. There is deliberately no PAUSED state: a restart resumes
// LIVE from the persisted wall-clock deadline instead (§6 crash recovery).
const (
	PhaseLobby     Phase = "LOBBY"
	PhaseItemIntro Phase = "ITEM_INTRO"
	PhaseLive      Phase = "LIVE"
	PhaseSettled   Phase = "SETTLED"
	PhaseFinale    Phase = "FINALE"
)

// phases is the declaration order the machine is defined in.
var phases = []Phase{PhaseLobby, PhaseItemIntro, PhaseLive, PhaseSettled, PhaseFinale}

// transitions is PLAN.md §6's complete legal edge set, including the edges the
// later phases will drive:
//
//	LOBBY      → ITEM_INTRO   admin item.start on the first item
//	ITEM_INTRO → LIVE         admin item.start
//	ITEM_INTRO → ITEM_INTRO   admin item.next / item.skip → the next item's intro
//	ITEM_INTRO → FINALE       item.next / item.skip with no item left
//	LIVE       → SETTLED      deadline reached (wall clock) or admin item.force_end
//	SETTLED    → ITEM_INTRO   admin item.next with items remaining
//	SETTLED    → FINALE       admin item.next with no item left
//	FINALE     → ITEM_INTRO   admin item.start — D-65 lists FINALE among the
//	                          phases that may open an item, because the presenter
//	                          can add an item during the finale. This is a
//	                          deliberate, test-visible widening of the §6 diagram
//	                          (a rendering, not the API): without it the command
//	                          D-65 promises would answer bad_message.
//	*          → LOBBY        admin game.reset — the reset arrow spans the whole
//	                          diagram in §6, so every row lists LOBBY
//
// Reaching LOBBY from any other phase is the one machine-wide edge; every
// other edge belongs to the item flow. A reset while already in LOBBY is a
// no-op rather than a self-edge, so LOBBY's row lists only ITEM_INTRO.
var transitions = map[Phase][]Phase{
	PhaseLobby:     {PhaseItemIntro},
	PhaseItemIntro: {PhaseLobby, PhaseLive, PhaseItemIntro, PhaseFinale},
	PhaseLive:      {PhaseLobby, PhaseSettled},
	PhaseSettled:   {PhaseLobby, PhaseItemIntro, PhaseFinale},
	PhaseFinale:    {PhaseLobby, PhaseItemIntro},
}

// AllPhases returns the phases in declaration order, as a copy: the machine is
// shared read-only state, and a caller appending to the result must not be able
// to widen it for everyone else.
func AllPhases() []Phase {
	return slices.Clone(phases)
}

// CanTransition reports whether the machine declares the from → to edge.
// An undeclared edge — including any phase name outside the catalog — is
// refused rather than guessed at.
func CanTransition(from, to Phase) bool {
	return slices.Contains(transitions[from], to)
}

// NextPhases returns the declared successors of from, as a copy (see
// AllPhases). An unknown phase has no successors.
func NextPhases(from Phase) []Phase {
	return slices.Clone(transitions[from])
}
