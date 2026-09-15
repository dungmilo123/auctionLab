package game

// This file is the engine's typed event vocabulary (D-57/D-58): what a mutation
// tells the fan-out edge to enqueue, in order, before the engine lock is
// released.
//
// game cannot import hub (hub imports game), so these literals are duplicated
// from internal/hub/protocol.go exactly like Role and the error codes are, and
// events_test.go pins them on both sides: renaming either side fails a test
// instead of silently moving the wire protocol (D-24).
//
// An Event carries no JSON tags of its own: `Data` is always one of project.go's
// projection payload types, which stay the only JSON-tagged types in the package
// (D-26's leak boundary).

// Event is one fan-out frame produced by a committed mutation. Type is the wire
// type; Data is the payload; To is empty for a broadcast and carries a player id
// for a private frame (D-53's `bid.rejected`).
//
// The zero Event — Type == EventSnapshot — means "send the recipient its own
// role projection": the snapshot is the sync point for join, resync and
// drop-recovery, and D-55/D-66 name the two mutations that end with one.
type Event struct {
	Type string
	Data any
	To   string
}

// The event types a Phase 3 mutation may produce. The values are the frozen
// wire strings (hub.Type*); `EventSnapshot` is the empty string by design (H1).
const (
	EventSnapshot     = ""               // the recipient's own role projection
	EventPhaseChanged = "phase.changed"  // {phase}
	EventItemIntro    = "item.intro"     // *PublicItem, status intro
	EventItemLive     = "item.live"      // *PublicItem with the live fields
	EventBidAccepted  = "bid.accepted"   // BidAccepted
	EventBidRejected  = "bid.rejected"   // BidRejection — private (To = player id)
	EventItemSettled  = "item.settled"   // *PublicItem with the settle object
	EventLeaderboard  = "leaderboard"    // []LeaderEntry
	EventPriceTick    = "price.tick"     // PriceTick — built by the hub (D-56), never bumps rev
)
