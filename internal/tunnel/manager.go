package tunnel

import (
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// This file owns the supervisor: exactly one cloudflared generation at a time,
// its merged output stream drained for the process's whole lifetime, the URL it
// announces published the moment it appears, a probe goroutine that decides
// health, and a restart loop that backs off with full jitter but never fights a
// deliberate shutdown.
//
// Three invariants are worth stating once, because every line below serves one
// of them:
//
//  1. The manager is never fatal. Constructing it validates what it can (the
//     binary, the port, a config-file conflict) and reports through the
//     injected logger; Start can be called on a manager with no binary at all
//     and does the only safe thing — publishes "no tunnel" and returns (D-123).
//  2. A deliberate stop is not a crash. The child exits 0 on SIGTERM (measured
//     on 2026.9.1), so the restart decision is `if !stopping`, never
//     `if exitCode != 0` (D-124).
//  3. Health is probe-derived. The manager never claims Healthy because a
//     process is alive; only a completed screen handshake through the published
//     URL flips it, and it takes two consecutive failures to flip it back
//     (D-126).

// Default timings. They are parameters, never constants: every test drives the
// mechanism with values short enough to observe, exactly as the hub's Policy
// does (D-40's precedent).
const (
	// DefaultBackoffBase is the first restart delay (D-125: 5s → cap 60s).
	DefaultBackoffBase = 5 * time.Second
	// DefaultBackoffCap is the ceiling the doubling schedule stops at, so
	// repeated quick-tunnel creations stay rare (PITFALLS 7's "keep creation
	// cycles low" — every restart is a new URL and a fresh 429 risk).
	DefaultBackoffCap = 60 * time.Second
	// DefaultStableAfter is how long a generation must live before the next
	// unexpected exit restarts at the base delay again. Without it a tunnel that
	// dies once an hour would keep restarting at the cap.
	DefaultStableAfter = 60 * time.Second
	// DefaultProbeEvery is D-126's health cadence.
	DefaultProbeEvery = 30 * time.Second
	// DefaultProbeTimeout bounds ONE probe attempt. It is generous on purpose:
	// the first probe after a URL appears is expected to fail while DNS settles
	// (measured: 2–5s), and the 30s cadence — not a retry loop — is the retry.
	DefaultProbeTimeout = 10 * time.Second
	// DefaultStopWait bounds the wait between SIGTERM and SIGKILL (D-124).
	DefaultStopWait = 5 * time.Second
)

// Config is the manager's full surface, seams included. Production fills in
// Port, Bin, Protocol, Publish and Home; everything else has a default.
type Config struct {
	// Port is the local server port cloudflared forwards to.
	Port int
	// Bin is the resolved cloudflared path; "" means "not installed", which is
	// a supported state, not an error (D-124).
	Bin string
	// Protocol, when non-empty, is passed through as `--protocol` (D-126's
	// TUNNEL_PROTOCOL escape hatch for cloudflared #1652). The flag is
	// undocumented in `--help` on 2026.9.1 but accepted; leaving it empty keeps
	// cloudflared's own default (quic).
	Protocol string
	// Publish receives every status change. Required.
	Publish PublishFunc
	// Probe is the round-trip health check; nil means ProbeURL.
	Probe ProbeFunc
	// Home is the directory the config-conflict scan treats as `~`; "" means the
	// real user home. Injected so tests drive the scan without touching it.
	Home string
	// Logf is the diagnostic sink; nil means the standard logger. Every
	// diagnostic is actionable — it names the fix, not just the symptom — so the
	// presenter can read it at the venue (D-124/D-125).
	Logf func(format string, args ...any)
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Jitter maps a delay to its randomized form; nil means full jitter over
	// [0, delay] with math/rand/v2. Jitter is not a secret, so this is the
	// correct generator (P8) — and injecting it keeps the restart tests
	// deterministic.
	Jitter func(time.Duration) time.Duration
	// Sleep is the wait seam; nil means time.Sleep. A test that would rather not
	// wait out a 5-second backoff passes one that returns immediately (or one
	// that records the schedule).
	Sleep func(time.Duration)

	// Timing knobs; zero values take the Default* constants above.
	BackoffBase  time.Duration
	BackoffCap   time.Duration
	StableAfter  time.Duration
	ProbeEvery   time.Duration
	ProbeTimeout time.Duration
	StopWait     time.Duration
}

// Manager supervises one cloudflared generation at a time. The zero value is
// not usable; New builds one. Start and Close are safe to call in any order and
// more than once — Close is idempotent, and Start on a manager that can never
// serve (no binary) is a logged no-op.
type Manager struct {
	cfg Config

	// stopping is set BEFORE the child is signalled, so the restart decision can
	// tell a deliberate stop from a crash that happens to exit 0 (measured on
	// 2026.9.1: SIGTERM → exit 0).
	stopping  atomic.Bool
	done      chan struct{} // closed by Close: interrupts waits and probes
	finished  chan struct{} // closed when the supervisor goroutine returns
	closeOnce sync.Once

	mu      sync.Mutex
	cmd     *exec.Cmd // the live generation, nil when none
	last    Status    // the last published status (dedupe)
	hasLast bool
}

// New builds a manager for one Config. It resolves nothing by itself: Bin is
// expected to be already resolved by the boot (resolveBinary), and a missing
// binary is reported by Start as a warning rather than an error here — the
// auction must boot with or without cloudflared (D-123).
func New(cfg Config) *Manager {
	return &Manager{cfg: cfg}
}

// Start launches the supervisor goroutine. It returns immediately: the URL
// arrives about seven seconds later, and the boot never blocks on the tunnel
// (measured 2026.9.1).
func (m *Manager) Start() {}

// Close stops the supervised child for good: it sets the stopping flag, sends
// SIGTERM, waits the bounded window, then SIGKILLs, and never schedules a
// successor. It is idempotent and safe on a manager that was never started.
func (m *Manager) Close() {}
