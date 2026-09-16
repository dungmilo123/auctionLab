package game

import "testing"

// This file is the literal contract lock for the event vocabulary, the same
// pattern identity_test.go uses for roles and error codes: game cannot import
// hub (hub imports game), so the strings are duplicated on purpose — and both
// sides pin them against literals in their own tests, so renaming either side
// fails a test instead of silently moving the wire protocol (D-24).

// TestEventLiteralsMatchTheHubCatalog pins every event type the engine may emit
// against the frozen catalog (hub.Type*).
func TestEventLiteralsMatchTheHubCatalog(t *testing.T) {
	types := map[string]string{
		EventPhaseChanged: "phase.changed",
		EventItemIntro:    "item.intro",
		EventItemLive:     "item.live",
		EventBidAccepted:  "bid.accepted",
		EventBidRejected:  "bid.rejected",
		EventItemSettled:  "item.settled",
		EventLeaderboard:  "leaderboard",
		EventPriceTick:    "price.tick",
		EventFinale:       "finale",
	}
	for got, want := range types {
		if got != want {
			t.Errorf("event type %q, want %q (hub.Type*)", got, want)
		}
	}

	// The zero Event is the snapshot marker, not a wire type: the fan-out edge
	// reads Type == "" as "send this client its own role projection" (H1).
	if EventSnapshot != "" {
		t.Errorf("EventSnapshot = %q, want the empty string", EventSnapshot)
	}
}

// TestBidReasonLiteralsMatchTheUiSpec pins D-53's payload-level reasons. They are
// NOT catalog error codes — the catalog keeps its seven values — but they are
// wire strings the client maps to copy (03-UI-SPEC reason.*), so a rename here
// would silently break every rejection message.
func TestBidReasonLiteralsMatchTheUiSpec(t *testing.T) {
	reasons := map[string]string{
		ReasonInvalidAmount:  "invalid_amount",
		ReasonWindowClosed:   "window_closed",
		ReasonBelowMinimum:   "below_minimum",
		ReasonExceedsBudget:  "exceeds_budget",
		ReasonAlreadyLeading: "already_leading",
	}
	for got, want := range reasons {
		if got != want {
			t.Errorf("bid reason %q, want %q (03-UI-SPEC reason.*)", got, want)
		}
	}
}
