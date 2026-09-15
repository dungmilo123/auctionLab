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
}

// DefaultPolicy carries D-40's production numbers: 64 buffered messages, a
// 4 KiB frame cap, a 60 s pong window refreshed by a ping every 25 s, and a
// 10 s write deadline.
var DefaultPolicy = Policy{
	SendBuffer: 64,
	ReadLimit:  4 << 10,
	PongWait:   60 * time.Second,
	PingPeriod: 25 * time.Second,
	WriteWait:  10 * time.Second,
}
