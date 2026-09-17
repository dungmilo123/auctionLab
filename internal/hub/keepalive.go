package hub

import "time"

// Policy holds one connection's liveness timings and queue sizing (D-40). The
// values are parameters, never constants: the mechanism is what the tests pin,
// and waiting out a real 60-second pong window would make them unusable.
//
// These numbers are deliberately larger than the library examples — a phone
// behind a tunnel pauses for seconds at a time — and well inside Cloudflare's
// roughly 100-second idle kill for a quick tunnel (research Pitfall 7/13).
// 02-03 consumes SendBuffer as the per-client channel depth; this tracer does
// not have a queue yet.
type Policy struct {
	SendBuffer int           // per-client buffered channel depth (D-41 drop/resync fence)
	ReadLimit  int64         // maximum client frame size, in bytes
	PongWait   time.Duration // silence longer than this means the peer is gone
	PingPeriod time.Duration // ping cadence; must stay well under PongWait
	WriteWait  time.Duration // bound on a single envelope or control write
	TickPeriod time.Duration // D-56 price.tick cadence; the ticker's only timing knob

	// HeartbeatPeriod is D-132's application-level beat: the cadence at which
	// every joined connection receives a `heartbeat` frame the BROWSER can
	// timestamp (it cannot see gorilla's ping frames). It is a separate knob from
	// PingPeriod on purpose — the two mechanisms solve different problems and the
	// tests inject a tiny period for one without touching the other.
	HeartbeatPeriod time.Duration

	// CommandRate and CommandBurst are D-138's inbound bucket: the sustained
	// commands per second one connection may send, and how many may arrive back
	// to back. They are deliberately generous — a false refusal on a legitimate
	// rapid-fire bid is a correctness bug, not a tuning preference — and the
	// acceptance loadtest is what validates the sizing from measured data.
	// A zero (or negative) pair disables the bucket.
	CommandRate  float64
	CommandBurst int
}

// DefaultPolicy carries D-40's production numbers: 64 buffered messages, a
// 4 KiB frame cap, a 60 s pong window refreshed by a ping every 25 s, a 10 s
// write deadline, and D-56's 250 ms tick cadence.
//
// Phase 7 (D-132/D-138) adds the two control-plane knobs: a 25 s heartbeat —
// the same cadence as the ping, so the client's 60 s staleness window is 2.4
// periods — and the 20/s, burst-40 inbound bucket.
var DefaultPolicy = Policy{
	SendBuffer: 64,
	ReadLimit:  4 << 10,
	PongWait:   60 * time.Second,
	PingPeriod: 25 * time.Second,
	WriteWait:  10 * time.Second,
	TickPeriod: 250 * time.Millisecond,

	HeartbeatPeriod: 25 * time.Second,
	CommandRate:     20,
	CommandBurst:    40,
}
