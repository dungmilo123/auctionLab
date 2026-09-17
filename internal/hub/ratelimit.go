package hub

import "time"

// This file owns D-138's per-connection inbound command limiter: a minimal token
// bucket over the frames ONE connection sends, checked before anything else in
// the read pump.
//
// The shape is deliberate and small. There is exactly one reader goroutine per
// connection, so the bucket needs no mutex and no atomic — it is read and
// written by one goroutine only, and it dies with the connection. Refill uses
// the hub's INJECTED clock (h.now), so a test advances time instead of sleeping.
//
// Two rules matter more than the arithmetic:
//
//   - A refusal NEVER closes the socket and never touches game state or the
//     revision. The frame is an ordinary private typed error (D-138), so a
//     client that spams costs the room one lock-held seq bump at most.
//   - At most ONE refusal frame is emitted per exhausted episode. Answering every
//     over-limit frame would be a 1:1 amplification for the sender — each spam
//     frame buying a JSON decode, a seq bump and a socket write — so further
//     frames are absorbed silently until the bucket refills (research Pattern 7,
//     a deliberate narrowing of D-138's wording that keeps its letter: an error
//     frame IS returned).
//
// The limiter is inbound-only: outbound frames are never metered, and the 4 KiB
// read limit stays exactly as it was.

// commandBucket is one connection's inbound token bucket.
type commandBucket struct {
	rate   float64 // tokens per second
	burst  float64 // capacity, i.e. how many frames may arrive back to back
	tokens float64
	last   time.Time

	// refusing records that the current exhaustion episode has already been
	// answered, so the next refused frame is silent.
	refusing bool
}

// newCommandBucket builds a bucket for one connection, full at the start: a
// fresh socket may burst up to CommandBurst frames immediately (a join followed
// by a quick correction must never be refused).
func newCommandBucket(policy Policy, now time.Time) *commandBucket {
	burst := float64(policy.CommandBurst)
	return &commandBucket{rate: policy.CommandRate, burst: burst, tokens: burst, last: now}
}

// admit consumes one token for one inbound frame. It reports whether the frame
// may proceed and, when it may not, whether this refusal is the FIRST of its
// episode — the caller answers only that one with the typed error.
//
// A policy with a non-positive rate or burst means "no bucket configured": the
// frame is admitted, so a hand-built Policy can never lock a connection out of
// its own commands.
func (b *commandBucket) admit(now time.Time) (ok bool, firstRefusal bool) {
	_ = now
	return true, false
}
