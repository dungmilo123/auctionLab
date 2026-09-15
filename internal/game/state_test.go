package game

import (
	"slices"
	"testing"
)

// TestPhaseMachine pins PLAN.md §6's machine: the declared phases, the declared
// edges, and that an undeclared edge is refused. It carries no revision notion
// of its own — the machine is a pure validator, and the single revision lives
// in the engine (D-35).
func TestPhaseMachine(t *testing.T) {
	if got, want := AllPhases(), []Phase{PhaseLobby, PhaseItemIntro, PhaseLive, PhaseSettled, PhaseFinale}; !slices.Equal(got, want) {
		t.Fatalf("AllPhases() = %v, want %v", got, want)
	}

	legal := []struct{ from, to Phase }{
		{PhaseLobby, PhaseItemIntro},
		{PhaseItemIntro, PhaseLive},
		{PhaseItemIntro, PhaseItemIntro},
		{PhaseItemIntro, PhaseFinale},
		{PhaseLive, PhaseSettled},
		{PhaseSettled, PhaseItemIntro},
		{PhaseSettled, PhaseFinale},
	}
	for _, edge := range legal {
		if !CanTransition(edge.from, edge.to) {
			t.Errorf("CanTransition(%s, %s) = false, want true", edge.from, edge.to)
		}
	}

	illegal := []struct{ from, to Phase }{
		{PhaseLobby, PhaseLobby}, // a reset in the lobby is a no-op, not an edge
		{PhaseLobby, PhaseLive},  // the item intro cannot be skipped
		{PhaseLobby, PhaseSettled},
		{PhaseLobby, PhaseFinale},
		{PhaseLive, PhaseItemIntro},
		{PhaseFinale, PhaseLive},
		{Phase("PAUSED"), PhaseLive}, // §6 has no PAUSED state
	}
	for _, edge := range illegal {
		if CanTransition(edge.from, edge.to) {
			t.Errorf("CanTransition(%s, %s) = true, want false", edge.from, edge.to)
		}
	}

	// The reset edge: §6's reset arrow spans the machine, so every other phase
	// can reach LOBBY (a reset while already in LOBBY is a no-op, not an edge).
	for _, from := range AllPhases() {
		if from == PhaseLobby {
			continue
		}
		if !CanTransition(from, PhaseLobby) {
			t.Errorf("CanTransition(%s, %s) = false, want true (game.reset returns any phase to LOBBY)", from, PhaseLobby)
		}
	}

	// The machine is shared read-only state: mutating a result must not widen it.
	next := NextPhases(PhaseLobby)
	if len(next) == 0 {
		t.Fatal("NextPhases(LOBBY) is empty")
	}
	next[0] = PhaseFinale
	if got := NextPhases(PhaseLobby); got[0] == PhaseFinale {
		t.Fatal("NextPhases exposes the shared table: a caller can widen the machine")
	}
}
