package hub

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
)

// This file is the ordering and convergence contract (RT-03 / RT-04, PLAN.md
// §3.1 #5): the fan-out runs inside the engine's critical section, a client that
// stops keeping up is fenced and repaired by ONE snapshot built under that same
// lock, and a client's own `resync` travels on the same FIFO as everything else
// — never out of band.

// maxRecoveryReads bounds the "read forward until the stream repairs itself"
// loops. A gorilla read deadline that expires leaves the connection permanently
// broken, so these cases read a known number of frames instead of waiting for a
// quiet socket.
const maxRecoveryReads = 64

// TestOrderingUnderBurst is research Pitfall 2's test: N concurrent mutations
// must reach one client as a gapless seq stream, in revision order, with no
// duplicate and no gap. It is red on an implementation that unlocks before
// broadcasting, which is the shape of nearly every WebSocket chat example.
func TestOrderingUnderBurst(t *testing.T) {
	// A deep FIFO: this case is about ORDER, not about overflow.
	h, _ := newTestHubWithPolicy(t, testPolicyWithBuffer(256))

	observer := dialWS(t, h)
	_, handshake := join(t, observer, `{"type":"join","role":"screen"}`)

	const burst = 12

	// Every connection is dialed up front: t.Fatalf is not allowed off the test
	// goroutine, so only the writes happen concurrently.
	conns := make([]*websocket.Conn, burst)
	for i := range conns {
		conns[i] = dialWS(t, h)
	}

	var wg sync.WaitGroup
	for i, conn := range conns {
		wg.Add(1)
		go func(i int, conn *websocket.Conn) {
			defer wg.Done()
			// Each join is a mutation: it bumps the revision and fans out.
			if err := writeFrameErr(conn, fmt.Sprintf(`{"type":"join","role":"player","nickname":"P%02d"}`, i)); err != nil {
				t.Errorf("burst join %d: %v", i, err)
			}
		}(i, conn)
	}
	wg.Wait()

	frames := make([]frame, 0, burst)
	for i := 0; i < burst; i++ {
		frames = append(frames, readFrame(t, observer))
	}

	// The stream continues exactly where the handshake pair left off: the first
	// fan-out frame is the next seq after the snapshot.
	if want := handshake.Seq + 1; frames[0].Seq != want {
		t.Fatalf("first fan-out frame seq = %d, want %d (the handshake pair owns seq 1 and 2)",
			frames[0].Seq, want)
	}
	for i := 1; i < len(frames); i++ {
		if got, want := frames[i].Seq, frames[i-1].Seq+1; got != want {
			t.Fatalf("frame %d: seq = %d, want %d — the stream has a gap or a duplicate (D-21/RT-03)\nframes: %s",
				i, got, want, frameSummary(frames))
		}
	}
	for i := 1; i < len(frames); i++ {
		if got, want := frames[i].Rev, frames[i-1].Rev+1; got != want {
			t.Fatalf("frame %d: rev = %d, want %d — a state event out of order or a revision skipped (RT-03)\nframes: %s",
				i, got, want, frameSummary(frames))
		}
	}
	if got := frames[len(frames)-1].Rev; got != burst {
		t.Errorf("last revision = %d, want %d (every mutation reached the observer)", got, burst)
	}
}

// TestStalledClientDoesNotBlockOthers is T-02-03's contract, and D-41's reason
// for existing: one phone that stops reading must not be able to slow the room
// down. The stalled connection is fenced and repaired; the healthy one keeps
// receiving every mutation, in order.
func TestStalledClientDoesNotBlockOthers(t *testing.T) {
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(2))
	inflateLobby(t, engine)

	stalled := dialStalled(t, h)
	stalledHello, _ := join(t, stalled, `{"type":"join","role":"player","nickname":"Stalled"}`)
	stalledID := frameString(t, frameData(t, stalledHello), "player")

	healthy := dialWS(t, h)
	_, base := join(t, healthy, `{"type":"join","role":"screen"}`)
	healthyReader := startReader(healthy)

	forceOverflow(t, h, engine, stalledID, 40)

	if h.Drops() == 0 {
		t.Fatal("no drop was recorded; the stalled peer's FIFO never overflowed, so D-41 was not exercised")
	}

	// The healthy client is read between mutations, so its own FIFO cannot fill:
	// whatever the stalled client is going through, this stream stays gapless.
	for i := 0; i < 5; i++ {
		conn := dialWS(t, h)
		if err := writeFrameErr(conn, fmt.Sprintf(`{"type":"join","role":"player","nickname":"H%02d"}`, i)); err != nil {
			t.Fatalf("mutation %d: %v", i, err)
		}
		env := healthyReader.next(t)
		if want := base.Seq + int64(i) + 1; env.Seq != want {
			t.Fatalf("healthy seq = %d at mutation %d, want %d — a stalled client delayed this client's stream",
				env.Seq, i, want)
		}
		if want := base.Rev + int64(i) + 1; env.Rev != want {
			t.Fatalf("healthy rev = %d at mutation %d, want %d", env.Rev, i, want)
		}
	}

	// The stalled client now catches up: it converges on the current revision
	// through a snapshot instead of replaying a burst it only half received.
	frames := drainFrames(t, stalled, 400*time.Millisecond)
	converged := false
	for _, env := range frames {
		if env.Type == TypeStateSnapshot && env.Rev == engine.Rev() {
			converged = true
		}
	}
	if !converged {
		t.Fatalf("the stalled client never saw a snapshot at the current revision %d; it cannot converge (D-41)\nframes: %s",
			engine.Rev(), frameSummary(frames))
	}
}

// TestGapConvergenceViaSnapshot is the phase's most valuable assertion (research
// Pitfall 3): a real overflow must repair through ONE snapshot built under the
// engine's exclusive lock and enqueued on the same FIFO — never written to the
// socket out of band, or the client would apply newer state and then regress.
func TestGapConvergenceViaSnapshot(t *testing.T) {
	h, engine := newTestHubWithPolicy(t, testPolicyWithBuffer(2))
	inflateLobby(t, engine)

	conn := dialStalled(t, h)
	hello, _ := join(t, conn, `{"type":"join","role":"player","nickname":"An"}`)
	id := frameString(t, frameData(t, hello), "player")

	before := h.Resyncs()
	forceOverflow(t, h, engine, id, 40)

	// Read forward until the stream shows the gap the drops left behind. The
	// reads are bounded: a quiet socket would otherwise have to be waited out,
	// and a gorilla read timeout kills the connection.
	frames := make([]frame, 0, 16)
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
		t.Fatalf("no seq gap appeared in %d frames: the forced overflow was not exercised\nframes: %s",
			maxRecoveryReads, frameSummary(frames))
	}
	if got := frames[recovery].Type; got != TypeStateSnapshot {
		t.Fatalf("the frame after the seq gap is %q, want %q — recovery must be a snapshot (D-41)\nframes: %s",
			got, TypeStateSnapshot, frameSummary(frames))
	}
	if got := h.Resyncs(); got <= before {
		t.Errorf("resync counter = %d, want it past %d (a drop must be followed by a recovery snapshot)", got, before)
	}
	if got := h.Drops(); got == 0 {
		t.Error("drop counter = 0 after a forced overflow (D-41 accounting)")
	}

	snapshot := frames[recovery]

	// A mutation after the recovery reaches this connection at a strictly newer
	// revision: the stream is live again, not merely repaired on paper. This is
	// also the assertion that no frame after the snapshot can be at or below its
	// revision — a post-recovery state event is the next revision by definition.
	late := dialWS(t, h)
	writeFrame(t, late, `{"type":"join","role":"player","nickname":"Binh"}`)
	next := readType(t, conn, TypeStateSnapshot)
	if next.Rev <= snapshot.Rev {
		t.Errorf("the first snapshot after the recovery carries rev %d, want it past the recovery's %d",
			next.Rev, snapshot.Rev)
	}
}

// TestSiblingMessagesShareARevision pins the rule that makes seq and rev two
// different fields (D-21, research Pitfall 7): one revision legitimately carries
// several messages, and a client that deduped on rev would silently discard
// every sibling. Both messages must arrive, in order, with consecutive seq.
func TestSiblingMessagesShareARevision(t *testing.T) {
	h, engine := newTestHub(t)

	conn := dialWS(t, h)
	join(t, conn, `{"type":"join","role":"screen"}`)

	// One critical section, one revision, two messages — the shape Phase 3's
	// `phase.changed` + `leaderboard` pair will have.
	if err := engine.Section(func(v game.View) error {
		h.Enqueue(v.Rev(), v)
		h.broadcast(v.Rev(), TypeLeaderboard, map[string]any{"standings": []any{}})
		return nil
	}); err != nil {
		t.Fatalf("driving one revision with two messages: %v", err)
	}

	first := readFrame(t, conn)
	second := readFrame(t, conn)

	if first.Type != TypeStateSnapshot || second.Type != TypeLeaderboard {
		t.Fatalf("siblings = %q then %q, want %q then %q", first.Type, second.Type, TypeStateSnapshot, TypeLeaderboard)
	}
	if first.Rev != second.Rev {
		t.Fatalf("sibling revs = %d then %d, want one revision (D-21)", first.Rev, second.Rev)
	}
	if second.Seq != first.Seq+1 {
		t.Errorf("sibling seqs = %d then %d, want consecutive numbers — gap detection is seq's job, never rev's",
			first.Seq, second.Seq)
	}
}

// TestResyncReturnsRoleSnapshot pins D-42's client-facing half: `resync` arrives
// through the connection's own FIFO, carries the role-appropriate projection of
// the CURRENT state, and takes a fresh seq like any other message.
//
// The order is the assertion, not an accident: the reply is queued behind the
// fan-out that was already on its way, so the snapshot can neither jump the
// queue nor fall behind it.
func TestResyncReturnsRoleSnapshot(t *testing.T) {
	h, _ := newTestHub(t)

	player := dialWS(t, h)
	hello, _ := join(t, player, `{"type":"join","role":"player","nickname":"An"}`)
	id := frameString(t, frameData(t, hello), "player")

	// The lobby moves on while the resync is in flight.
	other := dialWS(t, h)
	join(t, other, `{"type":"join","role":"player","nickname":"Binh"}`)
	writeFrame(t, player, `{"type":"resync"}`)

	// Frame one is the fan-out of Binh's join, frame two is the reply.
	pending := readFrame(t, player)
	if pending.Type != TypeStateSnapshot {
		t.Fatalf("frame after the mutation = %q, want %q", pending.Type, TypeStateSnapshot)
	}
	first := readFrame(t, player)
	if got := meOf(t, first); got.ID != id {
		t.Errorf("resync snapshot me.id = %q, want %q — the reply must be this connection's own role projection (D-25/D-42)", got.ID, id)
	}
	if first.Seq <= pending.Seq {
		t.Errorf("resync snapshot seq = %d, want it past the queued fan-out's %d — the reply must not jump the FIFO (D-42)",
			first.Seq, pending.Seq)
	}
	if first.Rev < pending.Rev {
		t.Errorf("resync snapshot rev = %d, want it at or past the queued fan-out's %d", first.Rev, pending.Rev)
	}
	if got := len(lobbyOf(t, first)); got != 2 {
		t.Errorf("resync snapshot lobby has %d entries, want 2 (the snapshot is built under the lock, not from a stale cache)", got)
	}
}

// TestResyncCoalescesBeforeTheWriterRuns pins D-42's coalescing rule at the
// boundary it is defined at: a client that asks twice before the writer runs
// gets ONE snapshot, not two. The client is built directly because the point is
// the FIFO, not the transport.
func TestResyncCoalescesBeforeTheWriterRuns(t *testing.T) {
	h, _ := newTestHub(t)

	c := &client{
		hub:  h,
		send: make(chan Envelope, 8),
		done: make(chan struct{}),
	}
	c.setIdentity(RoleScreen, "")

	c.requestResync()
	c.requestResync()
	c.requestResync()
	h.applyResync(c)

	if got := len(c.send); got != 1 {
		t.Fatalf("three resync requests enqueued %d snapshots, want 1 (D-42: coalesce before the writer runs)", got)
	}
	if got := (<-c.send).Type; got != TypeStateSnapshot {
		t.Errorf("the coalesced frame is %q, want %q", got, TypeStateSnapshot)
	}
}

// TestReadPumpErrorsStayOrderedWithFanout is GAP 1's regression (RT-03, D-21,
// D-33, D-41). Two different producers write the SAME connection's seq counter
// and FIFO: the observed connection's own read pump (a private `error` answer to
// an unsupported command) and the fan-out of every other connection's committed
// mutation (`Engine.publishLocked → Hub.Enqueue`). Only the engine's exclusive
// lock makes those two producers race-free, so this case drives both at once and
// asserts the observer's stream stays gapless and duplicate-free.
//
// It is red on an implementation whose read pump enqueues without the lock:
// `go test -race` reports the concurrent access to client.seq, and two producers
// can hand the same seq to two different messages — which the browser gate
// (web/src/ws/gate.ts) then discards as a duplicate, silently losing a real
// state change.
//
// NOTE (measured): this case PASSES on the pre-fix implementation without the
// race detector — the gapless-seq assertion alone cannot see a lost update.
// `go test -race` (make test-race, D-43) is the detector; do not run this file
// through a non-race gate and call it evidence.
func TestReadPumpErrorsStayOrderedWithFanout(t *testing.T) {
	// A deep FIFO: this case is about the two producers sharing one sequence,
	// not about overflow, so nothing may be dropped here.
	h, _ := newTestHubWithPolicy(t, testPolicyWithBuffer(256))

	observer := dialWS(t, h)
	_, handshake := join(t, observer, `{"type":"join","role":"screen"}`)

	// The observer is drained for the whole case: an unread socket would stall
	// its writer and turn an ordering failure into a read timeout.
	stream := startReader(observer)

	// One frame arrives per producer event: every private error is answered on
	// the observer's own connection, and every committed join fans out exactly
	// one snapshot to it. The counts are here because the assertion is about the
	// INTERLEAVING of the two producers, not about how many messages were sent.
	const (
		errorFrames = 60
		joiners     = 12
	)

	// Every joining connection is dialed up front: t.Fatalf is not allowed off
	// the test goroutine, so only the writes happen concurrently.
	conns := make([]*websocket.Conn, joiners)
	for i := range conns {
		conns[i] = dialWS(t, h)
	}

	var producers sync.WaitGroup

	// Producer A: the observer's own read pump, answering unimplemented
	// commands with a private typed error. It is paced so it keeps running while
	// the joins below are in flight instead of finishing first.
	producers.Add(1)
	go func() {
		defer producers.Done()
		for i := 0; i < errorFrames; i++ {
			if err := writeFrameErr(observer, fmt.Sprintf(`{"type":"%s"}`, TypeBidPlace)); err != nil {
				t.Errorf("driving the observer's error producer: %v", err)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Producer B: concurrent player joins from separate connections. Each one is
	// a committed mutation whose fan-out runs from a DIFFERENT connection's read
	// pump than producer A's.
	for i, conn := range conns {
		producers.Add(1)
		go func(i int, conn *websocket.Conn) {
			defer producers.Done()
			time.Sleep(time.Duration(i) * 5 * time.Millisecond)
			if err := writeFrameErr(conn, fmt.Sprintf(`{"type":"join","role":"player","nickname":"R%02d"}`, i)); err != nil {
				t.Errorf("concurrent join %d: %v", i, err)
			}
		}(i, conn)
	}

	producers.Wait()

	frames := make([]frame, 0, errorFrames+joiners)
	for i := 0; i < errorFrames+joiners; i++ {
		frames = append(frames, stream.next(t))
	}

	kinds := make(map[string]int, 2)
	previous := handshake
	for i, env := range frames {
		kinds[env.Type]++
		if got, want := env.Seq, previous.Seq+1; got != want {
			t.Fatalf("frame %d (%s): seq = %d, want %d — the private error producer and the fan-out do not share one lock-held sequence (D-21/D-33/RT-03)\nframes: %s",
				i, env.Type, got, want, frameSummary(frames))
		}
		previous = env
	}
	if got := kinds[TypeError]; got != errorFrames {
		t.Errorf("the observer saw %d %q frames, want %d — the private error producer did not run",
			got, TypeError, errorFrames)
	}
	if got := kinds[TypeStateSnapshot]; got != joiners {
		t.Errorf("the observer saw %d %q frames, want %d — the mutation fan-out did not run",
			got, TypeStateSnapshot, joiners)
	}

	// The stream is still live after the concurrent work: the next mutation
	// continues from where the mixed stream left off, at the next seq.
	late := dialWS(t, h)
	writeFrame(t, late, `{"type":"join","role":"player","nickname":"Late"}`)
	next := stream.next(t)
	if got, want := next.Seq, previous.Seq+1; got != want {
		t.Errorf("the frame after the concurrent work has seq %d, want %d — the stream did not continue gaplessly",
			got, want)
	}
}

// forceOverflow provokes a real overflow on one connection and returns how many
// messages were handed to it before the fence closed. The payload is a large
// synthetic blob rather than a projection because the point is VOLUME: the
// target's peer has a deliberately tiny receive buffer, so the writer blocks on
// the first write and cannot drain while the producer keeps enqueueing. The
// snapshot the connection is repaired with is the real role projection, and that
// is what the convergence assertions inspect.
func forceOverflow(t *testing.T, h *Hub, engine *game.Engine, playerID string, n int) {
	t.Helper()

	target := h.registry.lookup(playerID)
	if target == nil {
		t.Fatalf("no registered connection for %q", playerID)
	}
	pad := map[string]any{"pad": strings.Repeat("x", 1<<20)}
	if err := engine.Section(func(v game.View) error {
		for i := 0; i < n; i++ {
			target.enqueue(v.Rev(), TypeStateSnapshot, pad)
		}
		return nil
	}); err != nil {
		t.Fatalf("forcing an overflow: %v", err)
	}
}

// inflateLobby gives the projections real size before any client connects: the
// stalled-client case compares a real role projection against a real one, and a
// lobby of forty players is closer to the venue than an empty one. No client is
// connected yet, so these mutations fan out to nobody.
func inflateLobby(t *testing.T, engine *game.Engine) {
	t.Helper()

	for i := 0; i < 40; i++ {
		if _, err := engine.Join(game.JoinRequest{
			Role:     game.RolePlayer,
			Nickname: fmt.Sprintf("Filler%02d", i),
		}, nil); err != nil {
			t.Fatalf("inflating the lobby: %v", err)
		}
	}
}

// dialStalled connects a peer that stops reading, with a deliberately tiny
// receive buffer so the server's writes stop being absorbed after a couple of
// kilobytes. That is the real "phone that stopped reading" case made
// deterministic, instead of a race against the loopback socket's default
// buffering.
func dialStalled(t *testing.T, h *Hub) *websocket.Conn {
	t.Helper()

	srv := newTestServer(t, h)
	dialer := websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			d := net.Dialer{Control: func(_, _ string, c syscall.RawConn) error {
				var sockErr error
				if err := c.Control(func(fd uintptr) {
					sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, 2048)
				}); err != nil {
					return err
				}
				return sockErr
			}}
			return d.Dial(network, addr)
		},
	}

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := dialer.Dial(url, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
		}
		t.Fatalf("dialing a stalled peer at %s: %v (HTTP %d)", url, err, status)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// frameSummary renders the frames a failing case actually saw: the assertion is
// about the shape of the stream, and a bare count cannot show it.
func frameSummary(frames []frame) string {
	out := ""
	for _, env := range frames {
		out += fmt.Sprintf("\n  %s seq=%d rev=%d", env.Type, env.Seq, env.Rev)
	}
	if out == "" {
		return "(none)"
	}
	return out
}
