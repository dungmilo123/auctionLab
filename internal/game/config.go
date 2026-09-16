package game

// This file is D-81/D-84's config contract: `admin.config.update` is the ONE
// command that edits the persisted configuration, and the join URL the QR codes
// are rendered from is the same persisted server state (D-84) — a proposal list
// exists (D-83's enumeration, injected at boot) but the SELECTED value lives
// here, in game.config_json, because every consumer (admin panel, /screen
// projectors, a process that restarted mid-show) has to agree on it.

// ConfigInput is the `admin.config.update` payload (D-81/D-84): a partial
// update, so an absent field means "leave it alone". `starting_budget` is the
// Setup tab's budget field; `join_url` is the Setup tab's URL selection.
type ConfigInput struct {
	StartingBudget *int64  `json:"starting_budget"`
	JoinURL        *string `json:"join_url"`
}

// UpdateConfig is `admin.config.update` (D-81/D-84): an ordinary applyLocked
// mutation that edits the persisted configuration and nothing else.
//
// It deliberately never touches a player's balance — `starting_budget` is what
// the NEXT join is minted with and what a reset restores (D-78), so a mid-game
// edit cannot silently move anyone's economics.
func (e *Engine) UpdateConfig(in ConfigInput) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	next := e.state.config
	if in.StartingBudget != nil {
		next.StartingBudget = *in.StartingBudget
	}
	if in.JoinURL != nil {
		next.JoinURL = *in.JoinURL
	}

	return e.applyLocked(func(s *state) error {
		s.config = next
		// D-81/D-84: the revision moves and every recipient is handed its OWN
		// projection — the admin panel learns the new budget and selection, each
		// /screen projector rebuilds its QR code, and a player view is unchanged
		// because it has no field that could carry the change. The fan-out is
		// per-role by construction (hub.Enqueue calls v.Snapshot(role, …)), so
		// "admin-only" needs no narrower channel.
		s.events = append(s.events, Event{Type: EventSnapshot})
		return nil
	}, true, nil)
}

// selectedJoinURL is D-84's authoritative join URL: the persisted selection
// whenever one exists, and the first candidate only when the show has never
// selected one.
//
// The persisted value wins even when it is absent from the candidate list,
// because that is exactly the case D-86 creates: the admin may select
// `location.origin` (a dev box, a hotspot, a tunnel URL) and the server
// deliberately does not check it against its own enumeration. Re-defaulting
// because a URL is not in the list would silently undo the presenter's choice.
func (s *state) selectedJoinURL() string {
	if s.config.JoinURL != "" {
		return s.config.JoinURL
	}
	if len(s.joinURLCandidates) > 0 {
		return s.joinURLCandidates[0]
	}
	return ""
}
