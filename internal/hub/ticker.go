package hub

import (
	"log"
	"sync"
	"time"

	"auctionlab/internal/game"
)

// This file owns the D-56 ticker: ONE hub-level goroutine that beats at
// Policy.TickPeriod while an item is LIVE, settles a due English auction through
// the engine's mutation path, and otherwise broadcasts a `price.tick` built
// under the same lock every other producer uses.
//
// The split is deliberate: StartTicker is the production lifecycle (started and
// stopped by cmd/server), while tick is one deterministic beat a test can call
// directly — no test ever sleeps on the 250 ms production cadence (D-40's policy
// seam, H16).
//
// The ticker never settles anything itself. It DETECTS that the deadline passed
// and calls Engine.SettleIfDue, which re-checks the phase inside its own
// mutation, so a force-end that lands first makes the beat a clean no-op
// (Pitfall 2) and a tick can never write a budget.

// StartTicker starts the single tick goroutine and returns the stop function
// that ends it. The caller owns the lifetime: cmd/server defers it before the
// store closes, and every test fixture registers it with t.Cleanup, so a ticker
// can never outlive the store it reads (Pitfall 11). Stop is idempotent.
func (h *Hub) StartTicker() (stop func()) {
	beats := time.NewTicker(h.policy.TickPeriod)
	done := make(chan struct{})
	var once sync.Once

	go func() {
		for {
			select {
			case <-done:
				return
			case <-beats.C:
				h.tick()
			}
		}
	}()

	return func() {
		once.Do(func() {
			beats.Stop()
			close(done)
		})
	}
}

// tick is one beat. It settles first — a mutation with its own lock section —
// and only emits a timing frame if the run is still LIVE afterwards, so a due
// auction ends with the settle's own fan-out (item.settled + leaderboard +
// snapshot) rather than a stale LIVE tick (Pitfall 5).
//
// The settle arm runs even with nobody connected: the clock decides the outcome,
// not the audience (research Q7). The tick arm is skipped when the registry is
// empty because there is no one to tell.
func (h *Hub) tick() {
	settled, err := h.engine.SettleIfDue()
	if err != nil {
		// SettleIfDue reports a commit failure the engine already routed to
		// Fatal (D-34); the beat has nothing to add but the log line.
		log.Printf("tick: settle: %v", err)
		return
	}
	if settled {
		return
	}
	if len(h.registry.snapshot()) == 0 {
		return
	}

	_ = h.engine.Section(func(v game.View) error {
		payload, ok := v.Tick()
		if !ok {
			return nil // no open window: nothing to describe
		}
		h.broadcast(v.Rev(), TypePriceTick, payload) // the current revision, never a bump (D-58)
		return nil
	})
}
