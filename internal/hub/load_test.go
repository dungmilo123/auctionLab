package hub

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// This file owns REH-01's in-process half (D-135/D-136 layer 1): twenty real
// sockets through the real hub, rapid-firing real commands, every stream
// validated as it arrives, plus the deterministic stalled-client repair that
// proves the MECHANISM without depending on a socket buffer happening to fill.
//
// It extends ordering_test.go rather than forking it — dialStalled, forceOverflow,
// inflateLobby, testPolicyWithBuffer, readFrameErr and waitFor are the seams
// Phase 2 built for exactly this — and it asserts the two things the existing
// TestGapConvergenceViaSnapshot stops short of: the recovery snapshot carries the
// server's OWN revision, and a mutation after the repair reaches the repaired
// connection at that mutation's revision. A repair that left the client behind
// would be worse than the drop it replaced (D-41).
//
// Nothing here calls t.Fatalf off the test goroutine: the per-bot readers live in
// their own goroutines and report through values, because a FailNow from the
// wrong goroutine ends the wrong test.

// isStateEvent reports whether a frame carries a game revision that must advance
// contiguously (D-22). Control-plane frames (heartbeat, tunnel.status) are not
// state events, and neither are the frames that deliberately repeat the current
// revision (`price.tick`, D-58) or answer one connection privately
// (`bid.rejected`, D-53).
func isStateEvent(typ string) bool {
	switch typ {
	case TypeStateSnapshot, TypePhaseChanged, TypeItemIntro, TypeItemLive,
		TypeBidAccepted, TypeItemSettled, TypeLeaderboard, TypeFinale:
		return true
	}
	return false
}

// rapidBot is one bot in the twenty-client run: its socket, its own copy of the
// D-21/D-22 rules, and the accounting the convergence assertion reads. Every
// field is mutex-guarded because the reader goroutine owns the observations and
// the test goroutine owns the assertions.
type rapidBot struct {
	label string
	conn  *websocket.Conn

	mu       sync.Mutex
	want     int64
	lastRev  int64
	frames   int
	accepted int
	lastAt   time.Time // when the last frame arrived (the settle check)
	viol     error     // the first stream-contract violation, if any
	readErr  error     // why the reader stopped (expected at teardown)
}

// newRapidBot dials one bot and starts validating its stream from seq 1.
func newRapidBot(t *testing.T, h *Hub, index int) *rapidBot {
	t.Helper()

	b := &rapidBot{
		label:   fmt.Sprintf("bot %02d", index),
		conn:    dialWS(t, h),
		want:    1,
		lastRev: -1,
	}
	go b.read()
	return b
}

// read drains this bot's socket for the run's whole life. A read error ends the
// reader — including the one the test's own teardown causes — and convergence
// is what notices a bot that stopped early.
func (b *rapidBot) read() {
	for {
		env, err := readFrameErr(b.conn)
		if err != nil {
			b.mu.Lock()
			b.readErr = err
			b.mu.Unlock()
			return
		}
		if err := b.observe(env); err != nil {
			b.mu.Lock()
			if b.viol == nil {
				b.viol = err
			}
			b.mu.Unlock()
			return
		}
	}
}

// observe applies the frozen stream contract to one frame: seq contiguous from
// 1, a state event's revision contiguous (a repeat is a sibling, D-21/D-22), and
// the two control-plane frames counted without touching the revision.
func (b *rapidBot) observe(env frame) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lastAt = time.Now()

	if env.Type == TypeHeartbeat || env.Type == TypeTunnelStatus {
		// D-132/D-123: applied at any seq, never a state event. A stale copy has
		// already been counted, so it is ignored rather than treated as a
		// duplicate.
		if env.Seq >= b.want {
			b.want = env.Seq + 1
		}
		b.frames++
		return nil
	}

	if env.Seq < b.want {
		return fmt.Errorf("%s: duplicate seq %d — already carried (D-21)", b.label, env.Seq)
	}
	if env.Seq > b.want {
		return fmt.Errorf("%s: seq gap — the server sent %d, want %d (D-21)", b.label, env.Seq, b.want)
	}
	b.want++
	b.frames++

	if isStateEvent(env.Type) {
		if b.lastRev >= 0 && env.Rev != b.lastRev && env.Rev != b.lastRev+1 {
			return fmt.Errorf("%s: %s rev %d does not follow rev %d — revisions must be contiguous and never regress (D-22)",
				b.label, env.Type, env.Rev, b.lastRev)
		}
		b.lastRev = env.Rev
	}
	if env.Type == TypeBidAccepted {
		b.accepted++
	}
	return nil
}

// record keeps the first error a writer goroutine hit, so a failure inside the
// burst is reported by the test goroutine instead of being swallowed.
func (b *rapidBot) record(err error) {
	b.mu.Lock()
	if b.viol == nil {
		b.viol = err
	}
	b.mu.Unlock()
}

// waitForFrames blocks until the reader has validated at least n frames.
func (b *rapidBot) waitForFrames(t *testing.T, n int) {
	t.Helper()

	ok := waitFor(5*time.Second, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		return b.frames >= n
	})
	if ok {
		return
	}

	b.mu.Lock()
	frames, readErr := b.frames, b.readErr
	b.mu.Unlock()
	t.Fatalf("%s: only %d frames arrived (%v), want at least %d", b.label, frames, readErr, n)
}

// lastRevision is the last state revision this bot observed.
func (b *rapidBot) lastRevision() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastRev
}

// countAccepted is how many accepted bids this bot saw.
func (b *rapidBot) countAccepted() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.accepted
}

// quiet reports whether this bot's socket has been silent for at least d — the
// test's settle signal. The server processes the whole burst in microseconds, so
// a quiet window is what proves the reads have caught up with the writes before
// the final revision is sampled.
func (b *rapidBot) quiet(d time.Duration) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.lastAt.IsZero() && time.Since(b.lastAt) >= d
}

// requireClean fails the case for any stream-contract violation the reader
// recorded. A read error alone is not a violation: it is how every socket ends.
func (b *rapidBot) requireClean(t *testing.T) {
	t.Helper()

	b.mu.Lock()
	viol, frames, rev := b.viol, b.frames, b.lastRev
	b.mu.Unlock()
	if viol != nil {
		t.Errorf("%s: %v (after %d frames, last state rev %d)", b.label, viol, frames, rev)
	}
}

// TestTwentyClientsRapidFire is REH-01's in-process twenty-client proof (D-135):
// twenty real sockets join, every bot rapid-fires bids concurrently, and each
// bot's OWN stream is validated frame by frame — seq contiguous, a state event's
// revision contiguous, the control plane counted but never mistaken for state —
// and then all twenty must converge on the server's final revision.
//
// The FIFO is deliberately deep here: this case is about twenty-client ORDER
// under a real burst, not about overflow. The deterministic stalled-client case
// below owns the fence-and-repair mechanism.
func TestTwentyClientsRapidFire(t *testing.T) {
	const (
		clients  = 20
		commands = 8
	)

	// Test policy with a deep FIFO: 20 bots fan out 20 frames per mutation.
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(256))

	// One live item BEFORE anyone connects, so every burst command is a real
	// bid with a real fan-out (and its own frames go to nobody). Bids walk up by
	// one increment, so the room sees both producers the ordering contract must
	// keep straight: the accepted path (D-54, with its immediate tick) and the
	// private refusal (D-53).
	startLiveItem(t, engine, 60_000)

	bots := make([]*rapidBot, clients)
	for i := range bots {
		bots[i] = newRapidBot(t, h, i)
	}

	for i, b := range bots {
		writeFrame(t, b.conn, fmt.Sprintf(`{"type":"join","role":"player","nickname":"R%02d"}`, i))
	}
	// A bot's own handshake pair (hello, then the role snapshot) is the proof
	// that ITS join was committed — which is the barrier the burst needs, since
	// a command from a socket the server has not joined yet is refused.
	for _, b := range bots {
		b.waitForFrames(t, 2)
	}

	var burst sync.WaitGroup
	for i, b := range bots {
		burst.Add(1)
		go func(i int, b *rapidBot) {
			defer burst.Done()
			for c := 0; c < commands; c++ {
				// The first command is the SAME minimum bid for every bot, so
				// the burst's first processed frame is a guaranteed acceptance;
				// the rest escalate and exercise the refused path (below_minimum,
				// already_leading, exceeds_budget) beside it.
				amount := int64(110_000)
				if c > 0 {
					amount = int64(110_000 + (i*commands+c)*10_000)
				}
				frame := fmt.Sprintf(`{"type":"bid.place","amount":%d}`, amount)
				if err := writeFrameErr(b.conn, frame); err != nil {
					b.record(fmt.Errorf("%s: sending bid %d: %w", b.label, amount, err))
					return
				}
			}
		}(i, b)
	}
	burst.Wait()

	// Settle: the writes above are on the wire, but the server's per-connection
	// read pumps work independently of this goroutine, so the burst is not
	// "processed" until every socket has gone quiet. Sampling the final revision
	// before that would let the convergence assertion pass on a revision the
	// room is about to leave.
	if !waitFor(20*time.Second, func() bool {
		for _, b := range bots {
			if !b.quiet(250 * time.Millisecond) {
				return false
			}
		}
		return true
	}) {
		t.Fatal("the twenty bots never stopped receiving frames: the burst did not settle")
	}
	settled := engine.Rev()

	// One more mutation after the burst fans a snapshot at the server's FINAL
	// revision to every bot, which is what turns convergence into an observable
	// fact instead of an inference from silence.
	late := dialWS(t, h)
	writeFrame(t, late, `{"type":"join","role":"player","nickname":"Sentinel"}`)
	if !waitFor(5*time.Second, func() bool { return engine.Rev() > settled }) {
		t.Fatalf("the settling mutation never landed: revision stayed at %d", settled)
	}
	final := engine.Rev()

	if !waitFor(15*time.Second, func() bool {
		for _, b := range bots {
			if b.lastRevision() != final {
				return false
			}
		}
		return true
	}) {
		for i, b := range bots {
			if got := b.lastRevision(); got != final {
				t.Errorf("bot %02d converged on rev %d, want the room's final %d", i, got, final)
			}
		}
		t.Fatalf("the twenty bots did not converge on one revision within the budget (engine rev %d)", final)
	}

	for i, b := range bots {
		if got := b.lastRevision(); got != final {
			t.Errorf("bot %02d converged on rev %d, want the room's final %d", i, got, final)
		}
		b.requireClean(t)
	}

	// The burst really was a burst: the room accepted at least one bid, so the
	// convergence above is not the convergence of an idle room.
	if got := bots[0].countAccepted(); got == 0 {
		t.Error("no bid was accepted during the burst: the twenty-client run never exercised the mutation fan-out")
	}
	t.Logf("twenty clients, %d commands each, converged on revision %d", commands, final)
}

// TestStalledClientConvergesOnTheServerRevision is D-136 layer 1's deterministic
// proof: a peer whose FIFO is two deep and whose receive buffer is 2 KiB stops
// reading under a forced flood, is fenced, and is repaired — and the repair must
// land ON the server's revision, with a later mutation reaching it at that
// mutation's revision.
//
// The drill asserts the POSITIVE event (a gap repaired by the snapshot that
// reveals it) rather than merely "no error": a run in which nothing stalled
// proves nothing about the repair path, which is exactly what D-136 forbids.
func TestStalledClientConvergesOnTheServerRevision(t *testing.T) {
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(2))
	inflateLobby(t, engine)

	conn := dialStalled(t, h)
	hello, _ := join(t, conn, `{"type":"join","role":"player","nickname":"An"}`)
	id := frameString(t, frameData(t, hello), "player")

	before := h.Resyncs()
	forceOverflow(t, h, engine, id, 40)

	// Read forward until the gap appears: the frame that reveals it must be the
	// snapshot that repairs it (D-41). The reads are bounded because a gorilla
	// read deadline is terminal — waiting out a quiet socket would kill the very
	// connection under repair.
	frames := make([]frame, 0, maxRecoveryReads)
	recovery := -1
	for i := 0; i < maxRecoveryReads; i++ {
		env := readFrame(t, conn)
		if len(frames) > 0 && env.Seq > frames[len(frames)-1].Seq+1 {
			frames = append(frames, env)
			recovery = len(frames) - 1
			break
		}
		frames = append(frames, env)
	}
	if recovery < 0 {
		t.Fatalf("no seq gap appeared in %d frames: the forced overflow was never exercised\nframes: %s",
			maxRecoveryReads, frameSummary(frames))
	}

	snapshot := frames[recovery]
	if snapshot.Type != TypeStateSnapshot {
		t.Fatalf("the frame after the gap is %q, want %q — recovery must be a snapshot (D-41)\nframes: %s",
			snapshot.Type, TypeStateSnapshot, frameSummary(frames))
	}
	if snapshot.Rev != engine.Rev() {
		t.Errorf("the repair snapshot carries rev %d, want the server's own %d — a repair that left the client behind "+
			"would be worse than the drop it replaced (D-41)", snapshot.Rev, engine.Rev())
	}
	if got := h.Drops(); got == 0 {
		t.Error("drop counter = 0 after a forced overflow (D-41 accounting)")
	}
	if got := h.Resyncs(); got <= before {
		t.Errorf("resync counter = %d, want it past %d (a drop must be followed by a recovery snapshot)", got, before)
	}

	// Live again, not merely repaired on paper: the next mutation reaches this
	// connection at ITS revision.
	late := dialWS(t, h)
	writeFrame(t, late, `{"type":"join","role":"player","nickname":"Binh"}`)
	next := readType(t, conn, TypeStateSnapshot)
	if next.Rev != engine.Rev() {
		t.Errorf("the first snapshot after the repair carries rev %d, want the engine's moved %d", next.Rev, engine.Rev())
	}
}
