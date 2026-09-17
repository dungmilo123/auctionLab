package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
//     injected logger; Start on a manager with no binary does the only safe
//     thing — publishes "no tunnel" and returns (D-123). Nothing here can stop
//     the auction: game state is the only fatal thing in this program (D-34).
//  2. A deliberate stop is not a crash. The child exits 0 on SIGTERM (measured
//     on 2026.9.1), so the restart decision is `if !stopping`, never
//     `if exitCode != 0` (D-124).
//  3. Health is probe-derived. The manager never claims Healthy because a
//     process is alive; only a completed screen handshake through the published
//     URL flips it, and it takes two consecutive failures to flip it back
//     (D-126).
//
// The seams exist for exactly one reason: every rule above has a test that must
// not wait out a production cadence (the hub's D-40 Policy is the precedent).

// Default timings. They are parameters, never constants: every test drives the
// mechanism with values short enough to observe.
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

// maxLineBytes bounds one accumulated output line. cloudflared's lines are a
// few hundred bytes; the ceiling exists so a misbehaving child cannot make the
// supervisor allocate without limit (T-07-02's "bounded tails"), and a line
// past it ends the line-scanner without ending the drain.
const maxLineBytes = 64 * 1024

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
	// cloudflared's own default (quic). New normalizes it, so only quic and
	// http2 survive (NormalizeProtocol).
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
	// Sleep is the wait seam; nil means an interruptible timer. A test that
	// would rather not wait out a 5-second backoff passes one that returns
	// immediately (or one that records the schedule).
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
	started   atomic.Bool
	done      chan struct{} // closed by Close: interrupts waits and probes
	finished  chan struct{} // closed when the supervisor goroutine returns
	closeOnce sync.Once

	mu      sync.Mutex
	cmd     *exec.Cmd     // the live generation, nil when none
	genDone chan struct{} // closed when the live generation's Wait returns
	last    Status        // the last published status (dedupe)
	hasLast bool
}

// New builds a manager for one Config. It resolves nothing by itself: Bin is
// expected to be already resolved by the boot (resolveBinary), and a missing
// binary is reported by Start as a warning rather than an error here — the
// auction must boot with or without cloudflared (D-123).
func New(cfg Config) *Manager {
	if cfg.Publish == nil {
		cfg.Publish = func(Status) {}
	}
	if cfg.Probe == nil {
		cfg.Probe = ProbeURL
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Jitter == nil {
		cfg.Jitter = FullJitter
	}
	if cfg.BackoffBase <= 0 {
		cfg.BackoffBase = DefaultBackoffBase
	}
	if cfg.BackoffCap <= 0 {
		cfg.BackoffCap = DefaultBackoffCap
	}
	if cfg.StableAfter <= 0 {
		cfg.StableAfter = DefaultStableAfter
	}
	if cfg.ProbeEvery <= 0 {
		cfg.ProbeEvery = DefaultProbeEvery
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = DefaultProbeTimeout
	}
	if cfg.StopWait <= 0 {
		cfg.StopWait = DefaultStopWait
	}
	if cfg.Home == "" {
		if home, err := os.UserHomeDir(); err == nil {
			cfg.Home = home
		}
	}
	// The TUNNEL_PROTOCOL boundary (WR-02): a value cloudflared does not accept
	// is dropped here rather than passed through, so every caller — the boot
	// included — is covered by the one rule. The boot's own planTunnel logs the
	// dropped value so the operator learns why the option had no effect.
	cfg.Protocol = NormalizeProtocol(cfg.Protocol)
	return &Manager{
		cfg:      cfg,
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
}

// FullJitter is D-125's "full jitter": a uniform delay in the closed interval
// [0, delay]. Jitter is not a secret — math/rand/v2 is the right generator here
// (P8); crypto/rand is required for tokens, never for a restart schedule, and
// the injectable seam is what keeps the tests deterministic.
func FullJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(delay) + 1))
}

// The only `--protocol` values cloudflared accepts for a quick tunnel (verified
// on 2026.9.1). They are the whole vocabulary D-126's TUNNEL_PROTOCOL escape
// hatch may pass through.
const (
	ProtocolQUIC  = "quic"
	ProtocolHTTP2 = "http2"
)

// NormalizeProtocol returns the value to hand to `--protocol`, or "" to keep
// cloudflared's own default. Any other value is DROPPED here — the sink, so a
// caller that is not cmd/server cannot inject one either — because cloudflared
// exits on an unknown --protocol at every spawn: the supervisor would restart
// in a loop against Cloudflare's creation endpoint, which is exactly the
// 429-creating cycle PITFALLS 7 and D-125 exist to keep rare (07-RESEARCH.md's
// V5/command-injection rows: validate against the two known literals before
// use). The boot reports a dropped value in its own log line, next to the other
// environment names it resolves.
func NormalizeProtocol(protocol string) string {
	switch protocol {
	case ProtocolQUIC, ProtocolHTTP2:
		return protocol
	default:
		return ""
	}
}

// Start launches the supervisor goroutine. It returns immediately: the URL
// arrives about seven seconds later, and the boot never blocks on the tunnel
// (measured 2026.9.1). Calling Start twice is a no-op.
func (m *Manager) Start() {
	if !m.started.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer close(m.finished)
		m.run()
	}()
}

// Close stops the supervised child for good: it sets the stopping flag, sends
// SIGTERM, waits the bounded window, then SIGKILLs, and never schedules a
// successor. It is idempotent and safe on a manager that was never started.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		m.stopping.Store(true)
		close(m.done)
		m.terminate()
		if m.started.Load() {
			<-m.finished
		}
	})
}

// Status is the last value the manager published (the zero value before the
// first publish). The boot logs it, so "report the state actually observed"
// holds for the tunnel too.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.last
}

// publish is edge-triggered: the consumer fans one control-plane frame out to
// every connection and re-resolves the effective join URL, so identical values
// must not be re-sent. The callback runs OUTSIDE the lock — it may call back
// into this manager (or into a test's recorder) without deadlocking.
func (m *Manager) publish(status Status) {
	m.mu.Lock()
	if m.hasLast && m.last == status {
		m.mu.Unlock()
		return
	}
	m.last = status
	m.hasLast = true
	m.mu.Unlock()
	m.cfg.Publish(status)
}

// run is the supervisor loop: one generation at a time, forever, until Close.
func (m *Manager) run() {
	if path, found := ConfigConflict(m.cfg.Home); found {
		m.cfg.Logf("tunnel: quick tunnels are unsupported while %s exists; rename it "+
			"(cloudflared ignores --url while that file is present) — continuing, but expect no public URL", path)
	}
	if m.cfg.Bin == "" {
		m.cfg.Logf("tunnel: cloudflared is not installed; run scripts/install-cloudflared.sh, or point " +
			"--cloudflared-bin (or TUNNEL_BIN) at an existing binary — the auction continues without a public URL")
		m.publish(Status{})
		return
	}
	if m.cfg.Port <= 0 {
		m.cfg.Logf("tunnel: no local port to forward to (port=%d); the tunnel is disabled", m.cfg.Port)
		m.publish(Status{})
		return
	}

	attempt := 0
	for {
		lived, err, ran := m.runGeneration()
		if m.stopping.Load() {
			return
		}

		if !ran {
			// The binary is present but could not be spawned (not executable, a
			// bad interpreter, a vanished path). Back off and try again: the next
			// attempt may be after the presenter fixed it.
			delay := m.cfg.Jitter(backoffDelay(attempt, m.cfg.BackoffBase, m.cfg.BackoffCap))
			attempt++
			m.cfg.Logf("tunnel: starting %s: %v; retrying in %s", m.cfg.Bin, err, delay.Round(time.Millisecond))
			if !m.wait(delay) {
				return
			}
			continue
		}

		// The URL died with the child: publish the declared no-tunnel state so
		// the engine falls back to its persisted/LAN URL immediately (D-128).
		m.publish(Status{})

		if lived >= m.cfg.StableAfter {
			attempt = 0
		}
		delay := m.cfg.Jitter(backoffDelay(attempt, m.cfg.BackoffBase, m.cfg.BackoffCap))
		attempt++
		reason := "exit status 0"
		if err != nil {
			reason = err.Error()
		}
		m.cfg.Logf("tunnel: cloudflared exited (%s) after %s; restarting in %s",
			reason, lived.Round(time.Millisecond), delay.Round(time.Millisecond))
		if !m.wait(delay) {
			return
		}
	}
}

// wait sleeps for delay, returning false when the manager is stopping. With a
// real timer the wait is interruptible, so Close never sits out a 60-second
// backoff; the injected sleep seam is for tests that want to record the
// schedule without spending it.
func (m *Manager) wait(delay time.Duration) bool {
	if m.cfg.Sleep != nil {
		m.cfg.Sleep(delay)
		return !m.stopping.Load()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-m.done:
		return false
	case <-timer.C:
		return true
	}
}

// generation is one cloudflared process's observable state: the last URL it
// announced (woke is signalled on every announcement, so the probe fires
// immediately rather than waiting out a cadence) and whether it has already
// reported a throttled creation.
type generation struct {
	mu       sync.Mutex
	url      string
	woke     chan struct{}
	throttle atomic.Bool
}

func newGeneration() *generation { return &generation{woke: make(chan struct{}, 1)} }

// setURL records an announcement and wakes the probe loop.
func (g *generation) setURL(url string) {
	g.mu.Lock()
	g.url = url
	g.mu.Unlock()
	select {
	case g.woke <- struct{}{}:
	default: // a wake-up is already pending; the loop will read the newest URL
	}
}

// current is the URL the probe should test.
func (g *generation) current() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.url
}

// runGeneration spawns one cloudflared, supervises it until it exits, and
// reports how long it lived. `ran` is false when the process never started at
// all — the one failure that is not an exit.
//
// Two windows make a stop racing a spawn dangerous, and both are closed here
// rather than left to the caller: Close() sets `stopping` BEFORE it looks at
// `m.cmd`, so the flag is the one piece of state the two sides share. A stop
// that already landed refuses the spawn (window 1: `Start(); Close()` in quick
// succession); a stop that lands between `cmd.Start()` and the publish is
// answered by the re-check below, which finds the command already recorded
// (window 2). Without both, Close() would block on `<-m.finished` — which
// cannot close until the child exits — and a long-lived cloudflared would
// survive the process: the deferred mgr.Close in the boot would never return,
// and the SIGTERM/SIGKILL policy would be skipped entirely.
func (m *Manager) runGeneration() (lived time.Duration, waitErr error, ran bool) {
	if m.stopping.Load() {
		return 0, nil, false
	}

	args := []string{
		"tunnel",
		"--url", fmt.Sprintf("http://127.0.0.1:%d", m.cfg.Port),
		// --no-autoupdate is mandatory, not tidiness: cloudflared's own help says
		// the periodic check restarts the server with the new version, and a
		// restart mid-game rotates the URL under the audience's QR code.
		"--no-autoupdate",
	}
	if m.cfg.Protocol != "" {
		args = append(args, "--protocol", m.cfg.Protocol)
	}
	cmd := exec.Command(m.cfg.Bin, args...)

	// ONE pipe for both streams (D-124's merged drain) and, just as important,
	// ONE reader that lives as long as the process does. Reading only until the
	// first URL match would fill the pipe buffer and block cloudflared in
	// write(2) — the failure mode D-124 explicitly forbids.
	pr, pw, err := os.Pipe()
	if err != nil {
		return 0, err, false
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	started := m.cfg.Now()
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return 0, err, false
	}
	// The child holds its own descriptors now; the parent's write end must close
	// or the read end would never see EOF when the child exits.
	_ = pw.Close()

	genDone := make(chan struct{})
	m.mu.Lock()
	m.cmd = cmd
	m.genDone = genDone
	m.mu.Unlock()

	// The publish above is what terminate() reads, and `stopping` was set before
	// it looked — so a Close() that ran terminate() inside the window just now
	// found no cmd to signal and is waiting on genDone. One of the two sides
	// always sees the other: either terminate() saw this cmd, or this re-check
	// sees the flag and kills the child itself.
	if m.stopping.Load() {
		_ = cmd.Process.Kill()
	}

	gen := newGeneration()
	genCtx, genCancel := context.WithCancel(context.Background())
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		m.drain(pr, gen)
	}()
	go m.probeLoop(genCtx, gen)

	waitErr = cmd.Wait()
	lived = m.cfg.Now().Sub(started)

	m.mu.Lock()
	m.cmd = nil
	m.genDone = nil
	m.mu.Unlock()
	close(genDone)
	genCancel()

	// Unblock the drain even if a grandchild still holds the write end: the
	// reader must never outlive the generation.
	_ = pr.Close()
	<-drained

	return lived, waitErr, true
}

// drain reads the merged stream line by line for the process's whole lifetime.
// A line past maxLineBytes ends the scan; the remainder is discarded rather
// than left unread, because the one thing this goroutine may never do is stop
// emptying the pipe while the child is alive.
func (m *Manager) drain(r io.Reader, gen *generation) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		m.handleLine(scanner.Text(), gen)
	}
	if err := scanner.Err(); err != nil {
		m.cfg.Logf("tunnel: cloudflared output is no longer parsed (%v); draining it to keep the child unblocked", err)
	}
	_, _ = io.Copy(io.Discard, r)
}

// handleLine is the two things a line of cloudflared output can mean to this
// package: a newly announced URL, or a refused creation.
func (m *Manager) handleLine(line string, gen *generation) {
	if url, ok := scrapeURL([]byte(line)); ok {
		gen.setURL(url)
		// Published UNHEALTHY: the box is printed about a second before the edge
		// path is live, and the hostname is not resolvable for a few seconds after
		// that (measured). Healthy must be probe-derived, never optimistic.
		m.publish(Status{URL: url})
	}
	// Creation throttling (PITFALLS 7): restarting a quick tunnel too often gets
	// 429s from the creation endpoint. It is surfaced once per generation with
	// the action to take, and it never escalates — a rate-limited tunnel is an
	// unhealthy tunnel, not a broken auction.
	if isThrottleLine(line) && gen.throttle.CompareAndSwap(false, true) {
		m.cfg.Logf("tunnel: Cloudflare refused the quick-tunnel request (HTTP 429); creation is rate-limited " +
			"after repeated restarts — wait before restarting (the auction keeps running on LAN)")
		m.publish(Status{})
	}
}

// isThrottleLine recognises the 429 that the quick-tunnel creation endpoint
// returns after repeated restarts. The match is deliberately narrow: the number
// alone appears in enough unrelated output that keying on it would cry wolf.
func isThrottleLine(line string) bool {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "429") {
		return false
	}
	return strings.Contains(lower, "too many requests") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "quick tunnel")
}

// probeLoop decides health for one generation: probe immediately after every
// announcement, then on the cadence, and publish only on a state change. One
// failure is not a flap — it is the expected first probe while DNS settles — so
// the healthy=false transition waits for the second consecutive failure.
func (m *Manager) probeLoop(ctx context.Context, gen *generation) {
	failures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.done:
			return
		case <-gen.woke:
		case <-time.After(m.cfg.ProbeEvery):
		}

		url := gen.current()
		if url == "" {
			continue
		}

		probeCtx, cancel := context.WithTimeout(ctx, m.cfg.ProbeTimeout)
		err := m.cfg.Probe(probeCtx, url)
		cancel()

		if err != nil {
			failures++
			if failures == 1 {
				// Informational: the first failure after a URL change is expected
				// (DNS is NXDOMAIN for the first few seconds, measured). One log
				// line per incident, never one per 30-second beat.
				m.cfg.Logf("tunnel: probe %s failed: %v (one failure is not a flap; the next probe decides)", url, err)
			}
			if failures >= 2 {
				m.publish(Status{URL: url}) // Healthy: false
			}
			continue
		}

		failures = 0
		m.publish(Status{URL: url, Healthy: true})
	}
}

// terminate stops the live generation: SIGTERM, then the bounded wait, then
// SIGKILL. Callers set stopping first, so the supervisor loop treats the exit
// as deliberate and never schedules a successor.
func (m *Manager) terminate() {
	m.mu.Lock()
	cmd, genDone := m.cmd, m.genDone
	m.mu.Unlock()
	if cmd == nil || cmd.Process == nil || genDone == nil {
		return
	}

	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-genDone:
		return
	case <-time.After(m.cfg.StopWait):
	}

	m.cfg.Logf("tunnel: cloudflared did not stop within %s of SIGTERM; killing it", m.cfg.StopWait)
	_ = cmd.Process.Kill()
	select {
	case <-genDone:
	case <-time.After(m.cfg.StopWait):
		// The pid is gone either way; the supervisor loop is what reaps it.
	}
}
