package game

import "errors"

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

// UpdateConfig is `admin.config.update` (D-81/D-84): it edits the persisted
// configuration and nothing else — no player's budget moves, because the
// starting budget only affects joins that happen after it and the next reset.
func (e *Engine) UpdateConfig(in ConfigInput) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return errors.New("game: config.update is not implemented yet")
}
