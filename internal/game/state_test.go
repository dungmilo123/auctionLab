package game

import (
	"slices"
	"testing"
)

// This file is the literal contract lock for PLAN.md §6's state machine: the
// phase names and the edge table are written as STRINGS and production's
// constants must agree with them. A test that compared phase constants against
// phase constants could be satisfied by a rename; this one cannot.
//
// The machine owns no revision: CanTransition/NextPhases are pure predicates, so
// a rejected edge cannot move the single counter the engine owns (D-35). Only a
// successful, persisted mutation increments it.

// TestPhaseMachine pins the declared phases, every declared and undeclared edge,
// and the purity of the validator.
func TestPhaseMachine(t *testing.T) {
	// 1. The declared phases, in declaration order, as literals.
	want := []Phase{"LOBBY", "ITEM_INTRO", "LIVE", "SETTLED", "FINALE"}
	if got := AllPhases(); !slices.Equal(got, want) {
		t.Fatalf("AllPhases() = %v, want %v (PLAN.md §6)", got, want)
	}

	// 2. The edge table, literal → literal. The LOBBY column is §6's reset
	//    arrow, which spans the machine; the rest is the item flow.
	edges := []struct {
		from, to string
		legal    bool
	}{
		// legal — the item flow
		{"LOBBY", "ITEM_INTRO", true},      // admin item.start on the first item
		{"ITEM_INTRO", "LIVE", true},       // admin item.start
		{"ITEM_INTRO", "ITEM_INTRO", true}, // admin item.next / item.skip → next intro
		{"ITEM_INTRO", "FINALE", true},     // item.next / item.skip with no item left
		{"LIVE", "SETTLED", true},          // deadline reached, or admin item.force_end
		{"SETTLED", "ITEM_INTRO", true},    // admin item.next with items remaining
		{"SETTLED", "FINALE", true},        // admin item.next with no item left
		// legal — game.reset returns any phase to LOBBY
		{"ITEM_INTRO", "LOBBY", true},
		{"LIVE", "LOBBY", true},
		{"SETTLED", "LOBBY", true},
		{"FINALE", "LOBBY", true},
		// illegal
		{"LOBBY", "LOBBY", false}, // a reset in the lobby is a no-op, not an edge
		{"LOBBY", "LIVE", false},  // the item intro cannot be skipped
		{"LOBBY", "SETTLED", false},
		{"LOBBY", "FINALE", false},
		{"ITEM_INTRO", "SETTLED", false}, // only a live auction settles
		{"LIVE", "ITEM_INTRO", false},
		{"LIVE", "FINALE", false},
		{"SETTLED", "LIVE", false},
		{"FINALE", "ITEM_INTRO", false},
		{"FINALE", "LIVE", false},
		{"FINALE", "SETTLED", false},
		{"PAUSED", "LIVE", false},  // §6 has no PAUSED state
		{"LOBBY", "PAUSED", false}, // …and no phase can become one
	}
	for _, edge := range edges {
		if got := CanTransition(Phase(edge.from), Phase(edge.to)); got != edge.legal {
			t.Errorf("CanTransition(%s, %s) = %t, want %t", edge.from, edge.to, got, edge.legal)
		}
	}

	// 3. NextPhases must agree with the same table — one source of truth, no
	//    second list to drift — and must hand back a copy, so a caller cannot
	//    widen the machine for everyone else.
	for _, from := range AllPhases() {
		next := NextPhases(from)
		for _, to := range AllPhases() {
			if got, wantAgree := CanTransition(from, to), slices.Contains(next, to); got != wantAgree {
				t.Errorf("NextPhases(%s) disagrees with CanTransition(%s, %s): %t vs %t", from, from, to, wantAgree, got)
			}
		}
	}
	next := NextPhases(PhaseLobby)
	if !slices.Equal(next, []Phase{PhaseItemIntro}) {
		t.Fatalf("NextPhases(LOBBY) = %v, want [ITEM_INTRO]", next)
	}
	next[0] = PhaseFinale
	if got := NextPhases(PhaseLobby); !slices.Equal(got, []Phase{PhaseItemIntro}) {
		t.Fatalf("NextPhases(LOBBY) = %v after the caller mutated a previous result — the shared table leaked", got)
	}

	// 4. Purity: rejecting an illegal edge mutates nothing. The snapshot is
	//    taken around a full second sweep of the table above, so a validator
	//    that kept state would show up here (D-35: only a persisted mutation
	//    moves the revision, never a transition check).
	before := make([][]Phase, len(want))
	for i, phase := range want {
		before[i] = NextPhases(phase)
	}
	for _, edge := range edges {
		_ = CanTransition(Phase(edge.from), Phase(edge.to))
	}
	for i, phase := range want {
		if got := NextPhases(phase); !slices.Equal(got, before[i]) {
			t.Errorf("NextPhases(%s) changed across a read-only sweep: %v → %v", phase, before[i], got)
		}
	}
}
