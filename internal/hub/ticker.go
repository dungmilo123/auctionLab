package hub

// This file owns the D-56 ticker: ONE hub-level goroutine that beats at
// Policy.TickPeriod while an item is LIVE, settles a due English auction through
// the engine's mutation path, and otherwise broadcasts a `price.tick` built
// under the same lock every other producer uses.
//
// The split is deliberate: `StartTicker` is the production lifecycle (owned and
// stopped by cmd/server), while `tick` is one deterministic beat a test can call
// directly — no test ever sleeps on the 250 ms production cadence (D-40's policy
// seam, H16).

// StartTicker starts the single tick goroutine and returns the stop function
// that ends it. The caller owns the lifetime: cmd/server defers it before the
// store closes, and every test fixture registers it with t.Cleanup so a ticker
// can never outlive the store it reads (Pitfall 11).
//
// RED stub: the goroutine and its cadence are the behaviour under test.
func (h *Hub) StartTicker() (stop func()) {
	return func() {}
}

// tick is one beat. It settles first — a mutation, its own lock section — and
// only emits a timing frame when the run is still LIVE, so a due auction ends
// with the settle's own fan-out rather than a stale LIVE tick (Pitfall 5).
//
// RED stub: the settle-then-tick ordering is the behaviour under test.
func (h *Hub) tick() {}
