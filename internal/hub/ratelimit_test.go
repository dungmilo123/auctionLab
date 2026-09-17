package hub

import (
	"testing"
	"time"
)

// This file owns D-138's inbound command bucket at the transport boundary: a
// burst is admitted, the frame past it is answered with exactly ONE typed
// refusal, every further exhausted frame is absorbed silently, the socket stays
// open, game state and the revision do not move, and the bucket accepts again
// once the injected clock has refilled it.
//
// The clock is injected (h.now), so the refill is deterministic data rather than
// a sleep — the D-38 rule this package follows everywhere.

// TestCommandRateLimit is D-138's whole mechanism, driven through the real
// socket so the frame ordering is the contract it ships.
func TestCommandRateLimit(t *testing.T) {
	base := time.UnixMilli(1_700_000_000_000)
	clock := &testClock{t: base}
	policy := testPolicyWithBuffer(64)
	policy.CommandRate = 1
	// The join frame is metered like any other (D-138 exempts nothing), so a
	// burst of 3 is exactly: the join, the malformed frame, and one command.
	policy.CommandBurst = 3
	h, engine := newTestHubWithClock(t, policy, clock)

	conn := dialWS(t, h)
	join(t, conn, `{"type":"join","role":"screen"}`)

	rev := engine.Rev()

	// The burst: a MALFORMED frame and an unknown command are both metered, which
	// is the proof the bucket sits ahead of the decoder — neither may buy a free
	// answer.
	writeFrame(t, conn, "{not json")
	if env := readFrame(t, conn); env.Seq != 3 {
		t.Fatalf("the malformed frame was admitted at seq %d, want 3", env.Seq)
	} else {
		assertErrorCode(t, env, CodeBadMessage)
	}
	writeFrame(t, conn, `{"type":"future.command"}`)
	assertErrorCode(t, readFrame(t, conn), CodeUnsupported)

	// One frame past the burst: exactly one refusal, carrying the new code, at
	// the current revision.
	writeFrame(t, conn, `{"type":"future.command"}`)
	refusal := readFrame(t, conn)
	assertErrorCode(t, refusal, CodeRateLimited)
	if refusal.Rev != rev {
		t.Errorf("rate_limited rev = %d, want the unchanged %d (a refusal is not a state event, D-35)", refusal.Rev, rev)
	}
	if got := engine.Rev(); got != rev {
		t.Errorf("engine revision after the refusal = %d, want %d (the limiter mutates nothing)", got, rev)
	}

	// Every further exhausted frame is absorbed: the next thing on this socket is
	// the answer to the command sent AFTER the bucket refilled. If the silent
	// frames had been answered, this read would return another rate_limited.
	writeFrame(t, conn, `{"type":"future.command"}`)
	writeFrame(t, conn, `{"type":"future.command"}`)

	// The socket is still open and the injected clock refills the bucket: two
	// seconds at one token per second is a full burst again.
	clock.setTime(base.Add(2 * time.Second))
	writeFrame(t, conn, `{"type":"future.command"}`)

	after := readFrame(t, conn)
	assertErrorCode(t, after, CodeUnsupported)
	if after.Seq != refusal.Seq+1 {
		t.Errorf("the frame after the refill arrived at seq %d, want %d: a refused or absorbed frame must not consume this connection's sequence",
			after.Seq, refusal.Seq+1)
	}
	if after.Rev != rev {
		t.Errorf("the post-refill answer's rev = %d, want the unchanged %d", after.Rev, rev)
	}

	if got := h.Drops(); got != 0 {
		t.Errorf("hub drops = %d, want 0: the limiter absorbs frames before the FIFO, so it is not a D-41 drop", got)
	}
	if got := h.Resyncs(); got != 0 {
		t.Errorf("hub resyncs = %d, want 0", got)
	}
}
