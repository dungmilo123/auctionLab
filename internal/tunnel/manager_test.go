package tunnel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
	"auctionlab/internal/hub"
	"auctionlab/internal/store"
)

// This file is the D-123…D-126 supervisor contract at the transport boundary.
// Everything here drives a REAL process (a `/bin/sh` stub, the TUNNEL_BIN seam
// D-124 was shaped for) or a REAL WebSocket server (gorilla's DefaultDialer
// against an httptest hub) — the same "transport contracts, not mock contracts"
// rule the hub's own fixtures follow (D-43).
//
// Two rules keep the cases deterministic without weakening them:
//
//   - No case ever waits out a production cadence. The timing knobs in Config
//     exist for this file: backoff, probe interval and the SIGTERM window are
//     all injected as milliseconds.
//   - No case asserts on scheduler luck. Where a delay must be observed, the
//     Sleep seam records it, and the assertions are about the SEQUENCE
//     (base → cap → cap), never about wall-clock accuracy.

// statusBudget bounds every wait for a published status. It is generous enough
// for a `/bin/sh` spawn and short enough that a broken manager fails the case
// instead of hanging the package.
const statusBudget = 2 * time.Second

// testAdminKey is the production SHAPE (64 hex characters, D-74) with no real
// capability in it: the probe only ever joins as `screen`, which needs no key
// at all, and a fixture that could match a live server would be a leak.
const testAdminKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// --- fixtures ---------------------------------------------------------------

// statusRecorder is every status the manager published, in order, plus a
// channel for the cases that wait on one.
type statusRecorder struct {
	mu  sync.Mutex
	all []Status
	ch  chan Status
}

func newStatusRecorder() *statusRecorder {
	return &statusRecorder{ch: make(chan Status, 64)}
}

// publish is the PublishFunc the manager is built with.
func (r *statusRecorder) publish(s Status) {
	r.mu.Lock()
	r.all = append(r.all, s)
	r.mu.Unlock()
	select {
	case r.ch <- s:
	default: // a full channel means the case never read; the snapshot still has it
	}
}

// snapshot is everything published so far.
func (r *statusRecorder) snapshot() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Status(nil), r.all...)
}

// urls is the set of non-empty URLs published so far, in first-published order.
func (r *statusRecorder) urls() []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range r.snapshot() {
		if s.URL == "" || seen[s.URL] {
			continue
		}
		seen[s.URL] = true
		out = append(out, s.URL)
	}
	return out
}

// await returns the next published status matching want, or fails the case.
func (r *statusRecorder) await(t *testing.T, want func(Status) bool) Status {
	t.Helper()

	deadline := time.After(statusBudget)
	for {
		select {
		case s := <-r.ch:
			if want(s) {
				return s
			}
		case <-deadline:
			t.Fatalf("no matching tunnel status within %s; published: %v", statusBudget, r.snapshot())
			return Status{}
		}
	}
}

// awaitNewURL waits for a non-empty URL that has not been published before. The
// restart cases use it instead of comparing against one remembered URL: a
// generation publishes both its announcement and its healthy transition, so
// "different from the first" would be satisfied by the second frame of the same
// generation.
func (r *statusRecorder) awaitNewURL(t *testing.T) Status {
	t.Helper()

	seen := make(map[string]bool)
	for _, s := range r.snapshot() {
		if s.URL != "" {
			seen[s.URL] = true
		}
	}
	return r.await(t, func(s Status) bool { return s.URL != "" && !seen[s.URL] })
}

// assertQuiet fails the case when a new non-empty URL appears within d. It is
// how "Close never schedules a successor" is asserted without racing the
// manager's own shutdown publish.
func (r *statusRecorder) assertQuiet(t *testing.T, d time.Duration) {
	t.Helper()

	before := len(r.urls())
	time.Sleep(d)
	if after := len(r.urls()); after != before {
		t.Fatalf("a URL was published after the manager stopped: %v", r.urls())
	}
}

// logRecorder captures the manager's diagnostics. Assertions on the TEXT are
// deliberate: D-124/D-125 make "an actionable diagnostic" the deliverable, so
// the substrings checked below (the install script, TUNNEL_BIN, the conflicting
// path, 429) are the contract, not decoration.
type logRecorder struct {
	mu   sync.Mutex
	text strings.Builder
}

func (l *logRecorder) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(&l.text, format+"\n", args...)
}

func (l *logRecorder) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

func (l *logRecorder) contains(substr string) bool { return strings.Contains(l.all(), substr) }

// stubBin writes an executable POSIX shell stub and returns its path: the
// TUNNEL_BIN seam (D-124), the only way this package can drive a real process
// lifecycle without a real cloudflared or a network.
func stubBin(t *testing.T, dir, body string) string {
	t.Helper()

	path := filepath.Join(dir, "cloudflared-stub")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("writing the stub binary: %v", err)
	}
	return path
}

// urlLine is the measured 2026.9.1 shape: the URL inside a zerolog ASCII box on
// stderr. The scrape rule must not depend on the box, but the fixture should
// still look like the real thing.
func urlLine(host string) string {
	return `echo "ERR |  https://` + host + `.trycloudflare.com  |" >&2`
}

// baseConfig is the smallest working configuration: a temp home (so the
// config-conflict scan finds nothing) and a logger.
func baseConfig(t *testing.T, rec *statusRecorder, logs *logRecorder) Config {
	t.Helper()

	return Config{
		Port:    8080,
		Publish: rec.publish,
		Logf:    logs.logf,
		Home:    t.TempDir(),
	}
}

// --- TestManagerRestart -----------------------------------------------------

// TestManagerRestart is D-124/D-125's lifecycle contract: one generation at a
// time, every announced URL published, unexpected exits restarted on a jittered
// schedule, and a deliberate Close that ends the loop for good.
func TestManagerRestart(t *testing.T) {
	t.Run("publishes each announced URL and backs off between generations", func(t *testing.T) {
		dir := t.TempDir()
		count := filepath.Join(dir, "count")
		bin := stubBin(t, dir, fmt.Sprintf(`
n=0
[ -f %[1]q ] && n=$(cat %[1]q)
n=$((n+1))
printf '%%s' "$n" > %[1]q
%s
exit 1
`, count, urlLine("stub-$n")))

		rec := newStatusRecorder()
		logs := &logRecorder{}

		var mu sync.Mutex
		var delays []time.Duration
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.Probe = func(context.Context, string) error { return nil }
		cfg.ProbeEvery = time.Hour // health is not this case's subject
		cfg.BackoffBase = 20 * time.Millisecond
		cfg.BackoffCap = 40 * time.Millisecond
		cfg.StableAfter = time.Hour // never stable: the schedule must grow
		cfg.Jitter = func(d time.Duration) time.Duration { return d }
		cfg.Sleep = func(d time.Duration) {
			mu.Lock()
			delays = append(delays, d)
			mu.Unlock()
			time.Sleep(d)
		}

		m := New(cfg)
		m.Start()
		t.Cleanup(m.Close)

		first := rec.await(t, func(s Status) bool { return strings.HasPrefix(s.URL, "https://stub-") })
		if first.Healthy {
			t.Fatalf("a freshly announced URL must not be healthy before the probe answers: %+v", first)
		}
		rec.awaitNewURL(t)
		rec.awaitNewURL(t)

		mu.Lock()
		got := append([]time.Duration(nil), delays...)
		mu.Unlock()
		if len(got) < 2 {
			t.Fatalf("expected at least two restart delays, got %v", got)
		}
		if got[0] != 20*time.Millisecond {
			t.Fatalf("first restart delay = %s, want the 20ms base (D-125)", got[0])
		}
		if got[1] != 40*time.Millisecond {
			t.Fatalf("second restart delay = %s, want the 40ms cap (D-125)", got[1])
		}
		for i, d := range got {
			if d > 40*time.Millisecond {
				t.Fatalf("restart delay %d = %s, above the 40ms cap", i, d)
			}
		}
	})

	t.Run("a generation that outlives the stability threshold resets the backoff", func(t *testing.T) {
		dir := t.TempDir()
		count := filepath.Join(dir, "count")
		bin := stubBin(t, dir, fmt.Sprintf(`
n=0
[ -f %[1]q ] && n=$(cat %[1]q)
n=$((n+1))
printf '%%s' "$n" > %[1]q
%s
exit 0
`, count, urlLine("stable-$n")))

		rec := newStatusRecorder()
		logs := &logRecorder{}

		var mu sync.Mutex
		var delays []time.Duration
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.Probe = func(context.Context, string) error { return nil }
		cfg.ProbeEvery = time.Hour
		cfg.BackoffBase = 20 * time.Millisecond
		cfg.BackoffCap = 40 * time.Millisecond
		cfg.StableAfter = time.Nanosecond // every generation counts as stable
		cfg.Jitter = func(d time.Duration) time.Duration { return d }
		cfg.Sleep = func(d time.Duration) {
			mu.Lock()
			delays = append(delays, d)
			mu.Unlock()
			time.Sleep(d)
		}

		m := New(cfg)
		m.Start()
		t.Cleanup(m.Close)

		rec.await(t, func(s Status) bool { return strings.HasPrefix(s.URL, "https://stable-") })
		rec.awaitNewURL(t)
		rec.awaitNewURL(t)

		mu.Lock()
		got := append([]time.Duration(nil), delays...)
		mu.Unlock()
		if len(got) < 2 {
			t.Fatalf("expected at least two restart delays, got %v", got)
		}
		if got[0] != 20*time.Millisecond || got[1] != 20*time.Millisecond {
			t.Fatalf("delays = %v, want the base every time after a stable generation (D-125)", got)
		}
	})

	t.Run("backoffDelay doubles from the base and stops at the cap", func(t *testing.T) {
		base, top := 5*time.Second, 60*time.Second
		want := []time.Duration{
			5 * time.Second,
			10 * time.Second,
			20 * time.Second,
			40 * time.Second,
			60 * time.Second,
			60 * time.Second,
		}
		for attempt, exp := range want {
			if got := backoffDelay(attempt, base, top); got != exp {
				t.Fatalf("backoffDelay(%d) = %s, want %s", attempt, got, exp)
			}
		}
	})

	t.Run("keeps draining after the first URL, so a late rotation still lands", func(t *testing.T) {
		// The filler is far larger than a pipe buffer (64 KiB on Linux), so a
		// supervisor that stopped reading after its first match would leave the
		// child blocked in write(2) and the second URL would never arrive
		// (D-124's explicit prohibition).
		dir := t.TempDir()
		bin := stubBin(t, dir, urlLine("first-host")+`
i=0
while [ $i -lt 4096 ]; do
  echo "INF filler filler filler filler filler filler filler filler filler filler" >&2
  i=$((i+1))
done
`+urlLine("second-host")+`
while :; do sleep 0.05; done
`)

		rec := newStatusRecorder()
		logs := &logRecorder{}
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.Probe = func(context.Context, string) error { return nil }
		cfg.ProbeEvery = time.Hour

		m := New(cfg)
		m.Start()
		t.Cleanup(m.Close)

		rec.await(t, func(s Status) bool { return strings.Contains(s.URL, "https://first-host.trycloudflare.com") })
		rec.await(t, func(s Status) bool { return strings.Contains(s.URL, "https://second-host.trycloudflare.com") })
	})

	t.Run("Close terminates the child and never schedules a successor", func(t *testing.T) {
		dir := t.TempDir()
		marker := filepath.Join(dir, "term")
		bin := stubBin(t, dir, fmt.Sprintf(`
trap 'printf term > %q; exit 0' TERM
%s
while :; do sleep 0.05; done
`, marker, urlLine("term-now")))

		rec := newStatusRecorder()
		logs := &logRecorder{}
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.Probe = func(context.Context, string) error { return nil }
		cfg.ProbeEvery = time.Hour
		cfg.StopWait = time.Second

		m := New(cfg)
		m.Start()

		rec.await(t, func(s Status) bool { return strings.HasPrefix(s.URL, "https://term") })

		done := make(chan struct{})
		go func() {
			m.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("Close did not return within the bounded stop window")
		}

		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("the child never received SIGTERM (marker missing): %v", err)
		}
		rec.assertQuiet(t, 200*time.Millisecond)
	})

	t.Run("kills a child that ignores SIGTERM after the bounded wait", func(t *testing.T) {
		dir := t.TempDir()
		beats := filepath.Join(dir, "beats")
		bin := stubBin(t, dir, fmt.Sprintf(`
trap '' TERM
%s
i=0
while [ $i -lt 200 ]; do
  printf x >> %q
  sleep 0.05
  i=$((i+1))
done
`, urlLine("hang-on"), beats))

		rec := newStatusRecorder()
		logs := &logRecorder{}
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.Probe = func(context.Context, string) error { return nil }
		cfg.ProbeEvery = time.Hour
		cfg.StopWait = 80 * time.Millisecond

		m := New(cfg)
		m.Start()
		rec.await(t, func(s Status) bool { return strings.HasPrefix(s.URL, "https://hang") })
		time.Sleep(200 * time.Millisecond) // let the child heartbeat a few times

		start := time.Now()
		m.Close()
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Fatalf("Close took %s; the SIGKILL fallback did not fire", elapsed)
		}

		size := fileSize(t, beats)
		time.Sleep(300 * time.Millisecond)
		if now := fileSize(t, beats); now != size {
			t.Fatalf("the child was still running after Close (%d → %d bytes)", size, now)
		}
		rec.assertQuiet(t, 150*time.Millisecond)
	})
}

// fileSize is the heartbeat file's length, or 0 before it exists.
func fileSize(t *testing.T, path string) int64 {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// --- TestManagerDegrades ----------------------------------------------------

// TestManagerDegrades is D-123/D-125's "never fatal" contract. Every case here
// removes something the tunnel needs and asserts the same outcome: an
// actionable line in the log, a status that says there is no usable tunnel, and
// a manager that still starts and stops cleanly.
func TestManagerDegrades(t *testing.T) {
	t.Run("missing binary degrades with an actionable diagnostic", func(t *testing.T) {
		rec := newStatusRecorder()
		logs := &logRecorder{}
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = ""

		m := New(cfg)
		m.Start()

		status := rec.await(t, func(s Status) bool { return s.URL == "" && !s.Healthy })
		if status.URL != "" || status.Healthy {
			t.Fatalf("status = %+v, want the declared no-tunnel state", status)
		}
		for _, want := range []string{"install-cloudflared.sh", "TUNNEL_BIN"} {
			if !logs.contains(want) {
				t.Fatalf("the missing-binary diagnostic does not mention %q:\n%s", want, logs.all())
			}
		}

		done := make(chan struct{})
		go func() {
			m.Close()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Close blocked on a manager with no binary")
		}
	})

	t.Run("a config-file conflict is reported with the offending path", func(t *testing.T) {
		home := t.TempDir()
		conflict := filepath.Join(home, ".cloudflared", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(conflict), 0o755); err != nil {
			t.Fatalf("creating the conflict directory: %v", err)
		}
		if err := os.WriteFile(conflict, []byte("tunnel: x\n"), 0o644); err != nil {
			t.Fatalf("writing the config file: %v", err)
		}

		rec := newStatusRecorder()
		logs := &logRecorder{}
		cfg := baseConfig(t, rec, logs)
		cfg.Home = home
		cfg.Bin = ""

		m := New(cfg)
		m.Start()
		t.Cleanup(m.Close)

		rec.await(t, func(s Status) bool { return s.URL == "" && !s.Healthy })

		if !logs.contains(conflict) {
			t.Fatalf("the conflict diagnostic does not name %s:\n%s", conflict, logs.all())
		}
	})

	t.Run("an unusable binary is retried with backoff instead of crashing", func(t *testing.T) {
		dir := t.TempDir()
		bin := filepath.Join(dir, "not-executable")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o644); err != nil {
			t.Fatalf("writing the unusable binary: %v", err)
		}

		rec := newStatusRecorder()
		logs := &logRecorder{}
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.BackoffBase = 10 * time.Millisecond
		cfg.BackoffCap = 20 * time.Millisecond
		cfg.Jitter = func(d time.Duration) time.Duration { return d }

		m := New(cfg)
		m.Start()
		t.Cleanup(m.Close)

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Count(logs.all(), bin) >= 2 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if got := strings.Count(logs.all(), bin); got < 2 {
			t.Fatalf("expected the spawn failure to be logged and retried, saw %d mention(s):\n%s", got, logs.all())
		}
		if !logs.contains("retrying") {
			t.Fatalf("the spawn diagnostic is not actionable:\n%s", logs.all())
		}
	})

	t.Run("a refused creation is surfaced as actionable throttling", func(t *testing.T) {
		dir := t.TempDir()
		bin := stubBin(t, dir, `
echo "ERR Failed to request quick Tunnel: Post \"https://api.trycloudflare.com/tunnel\": 429 Too Many Requests" >&2
exit 1
`)

		rec := newStatusRecorder()
		logs := &logRecorder{}
		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.BackoffBase = 10 * time.Millisecond
		cfg.BackoffCap = 20 * time.Millisecond
		cfg.Jitter = func(d time.Duration) time.Duration { return d }

		m := New(cfg)
		m.Start()
		t.Cleanup(m.Close)

		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && !logs.contains("429") {
			time.Sleep(10 * time.Millisecond)
		}
		if !logs.contains("429") || !logs.contains("rate-limited") {
			t.Fatalf("the throttling diagnostic is not actionable:\n%s", logs.all())
		}
		// No URL was ever announced, so the only honest status is "no tunnel".
		if got := rec.urls(); len(got) != 0 {
			t.Fatalf("a refused creation published a URL: %v", got)
		}
	})

	t.Run("health flips only after two consecutive probe failures", func(t *testing.T) {
		dir := t.TempDir()
		bin := stubBin(t, dir, urlLine("health-ok")+`
while :; do sleep 0.05; done
`)

		rec := newStatusRecorder()
		logs := &logRecorder{}

		var calls atomic.Int64
		probe := func(context.Context, string) error {
			// 1: healthy. 2 and 3: a single failure must not flap. 4: healthy again.
			if n := calls.Add(1); n == 1 || n == 4 {
				return nil
			}
			return errors.New("probe: dial tcp: no such host")
		}

		cfg := baseConfig(t, rec, logs)
		cfg.Bin = bin
		cfg.Probe = probe
		cfg.ProbeEvery = 5 * time.Millisecond
		cfg.ProbeTimeout = 200 * time.Millisecond

		m := New(cfg)
		m.Start()
		t.Cleanup(m.Close)

		announced := rec.await(t, func(s Status) bool { return strings.HasPrefix(s.URL, "https://health") })
		if announced.Healthy {
			t.Fatalf("the announcement must be unhealthy: %+v", announced)
		}
		healthy := rec.await(t, func(s Status) bool { return s.Healthy })
		if healthy.URL != announced.URL {
			t.Fatalf("healthy status named %q, want %q", healthy.URL, announced.URL)
		}
		unhealthy := rec.await(t, func(s Status) bool { return !s.Healthy && s.URL != "" })
		if unhealthy.URL != announced.URL {
			t.Fatalf("unhealthy status named %q, want %q", unhealthy.URL, announced.URL)
		}
		recovered := rec.await(t, func(s Status) bool { return s.Healthy })
		if recovered.URL != announced.URL {
			t.Fatalf("recovered status named %q, want %q", recovered.URL, announced.URL)
		}
	})

	t.Run("resolveBinary prefers the flag, then TUNNEL_BIN, then PATH", func(t *testing.T) {
		env := func(v string) func(string) string {
			return func(key string) string {
				if key == "TUNNEL_BIN" {
					return v
				}
				return ""
			}
		}
		lookPath := func(string) (string, error) { return "/usr/local/bin/cloudflared", nil }

		if got, ok := resolveBinary("/flag/cloudflared", env("/env/cloudflared"), lookPath); !ok || got != "/flag/cloudflared" {
			t.Fatalf("flag precedence: got (%q, %v)", got, ok)
		}
		if got, ok := resolveBinary("", env("/env/cloudflared"), lookPath); !ok || got != "/env/cloudflared" {
			t.Fatalf("env precedence: got (%q, %v)", got, ok)
		}
		if got, ok := resolveBinary("", env(""), lookPath); !ok || got != "/usr/local/bin/cloudflared" {
			t.Fatalf("PATH fallback: got (%q, %v)", got, ok)
		}
		if _, ok := resolveBinary("", env(""), func(string) (string, error) { return "", errors.New("not found") }); ok {
			t.Fatal("a binary that resolves nowhere must report false")
		}
	})
}

// --- TestProbe --------------------------------------------------------------

// TestProbe is D-126's health primitive: the exact screen handshake, the
// failure classifications worth distinguishing, and the proof that a probe
// cycle is invisible to the game.
func TestProbe(t *testing.T) {
	t.Run("completes the screen handshake without touching game state", func(t *testing.T) {
		f := newHubFixture(t)

		rev, players := f.engine.Rev(), f.engine.PlayerCount()
		rows := f.rowCounts(t)
		for i := 0; i < 3; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := ProbeURL(ctx, f.url)
			cancel()
			if err != nil {
				t.Fatalf("probe cycle %d: %v", i, err)
			}
		}

		if got := f.engine.Rev(); got != rev {
			t.Fatalf("revision moved across probe cycles: %d → %d (D-126: a screen join is not a mutation)", rev, got)
		}
		if got := f.engine.PlayerCount(); got != players {
			t.Fatalf("player count moved across probe cycles: %d → %d", players, got)
		}
		if after := f.rowCounts(t); after != rows {
			t.Fatalf("persisted rows moved across probe cycles: %v → %v", rows, after)
		}
		if got := f.hub.Drops(); got != 0 {
			t.Fatalf("probe cycles left %d dropped frame(s); the probe must read its two frames eagerly", got)
		}
	})

	t.Run("rejects a first frame that is not hello at seq 1", func(t *testing.T) {
		url := scriptedServer(t, func(c *websocket.Conn) {
			_, _, _ = c.ReadMessage()
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"state.snapshot","seq":1,"rev":0,"ts":1,"data":{}}`))
		})
		if err := probeOnce(url); err == nil {
			t.Fatal("a handshake that opens with state.snapshot must fail the probe")
		}
	})

	t.Run("rejects a snapshot that is not at seq 2", func(t *testing.T) {
		url := scriptedServer(t, func(c *websocket.Conn) {
			_, _, _ = c.ReadMessage()
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello","seq":1,"rev":0,"ts":1,"data":{"role":"screen"}}`))
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"state.snapshot","seq":3,"rev":0,"ts":1,"data":{}}`))
		})
		if err := probeOnce(url); err == nil {
			t.Fatal("a snapshot at the wrong sequence must fail the probe")
		}
	})

	t.Run("rejects an error frame", func(t *testing.T) {
		url := scriptedServer(t, func(c *websocket.Conn) {
			_, _, _ = c.ReadMessage()
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","seq":1,"rev":0,"ts":1,"data":{"code":"unsupported","message":"nope"}}`))
		})
		err := probeOnce(url)
		if err == nil {
			t.Fatal("an error frame must fail the probe")
		}
		if !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("the probe error does not name the code that refused it: %v", err)
		}
	})

	t.Run("reports a refused upgrade as a bad handshake", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "nope", http.StatusBadRequest)
		}))
		t.Cleanup(srv.Close)

		err := probeOnce(srv.URL)
		if err == nil {
			t.Fatal("an HTTP 400 on the upgrade must fail the probe")
		}
		if !strings.Contains(err.Error(), "400") {
			t.Fatalf("the #1652 signature (400) must be reported verbatim: %v", err)
		}
	})

	t.Run("times out when the server never answers", func(t *testing.T) {
		url := scriptedServer(t, func(c *websocket.Conn) {
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		})

		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		err := ProbeURL(ctx, url)
		if err == nil {
			t.Fatal("a silent server must fail the probe")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "deadline") &&
			!strings.Contains(strings.ToLower(err.Error()), "timeout") {
			t.Fatalf("the probe error does not report the timeout: %v", err)
		}
	})
}

// probeOnce runs one probe with a generous budget; the cases above assert on
// the failure, never on the clock.
func probeOnce(url string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return ProbeURL(ctx, url)
}

// scriptedServer is a real WebSocket endpoint whose answer to the join is
// whatever the case writes. It is an httptest server plus gorilla's upgrader —
// the same transport the hub serves, minus the hub.
func scriptedServer(t *testing.T, handle func(c *websocket.Conn)) string {
	t.Helper()

	var upgrader websocket.Upgrader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		handle(conn)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// hubFixture is the in-process twin of the real thing: a fresh engine on a temp
// database, the hub's own WebSocket handler on a real listener, and the
// fan-out edge installed exactly as cmd/server installs it. A probe against it
// goes over a real socket, which is the only way to assert the handshake
// contract.
type hubFixture struct {
	engine *game.Engine
	hub    *hub.Hub
	url    string
	db     *sql.DB
}

func newHubFixture(t *testing.T) *hubFixture {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "auction.db"))
	if err != nil {
		t.Fatalf("opening the test database: %v", err)
	}
	if err := store.Schema(st.DB); err != nil {
		t.Fatalf("applying the schema: %v", err)
	}

	engine, err := game.New(game.Config{
		Store: st,
		Now:   time.Now,
		Fatal: func(err error) { t.Errorf("engine reported an unrecoverable commit failure: %v", err) },
	})
	if err != nil {
		t.Fatalf("building the engine: %v", err)
	}

	h := hub.New(engine, testAdminKey)
	engine.SetPublisher(h.Fanout)

	// httptest does not count a hijacked connection as an outstanding request,
	// so the handler wait group is what makes the shutdown order deterministic —
	// the hub's own fixture makes the same point.
	var handlers sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Done()
		h.ServeWS(w, r)
	}))
	t.Cleanup(func() {
		srv.Close()
		handlers.Wait()
		_ = st.Close()
	})

	return &hubFixture{engine: engine, hub: h, url: srv.URL, db: st.DB}
}

// rowCounts is the persisted side of "the probe changed nothing".
func (f *hubFixture) rowCounts(t *testing.T) [3]int {
	t.Helper()

	var out [3]int
	for i, table := range []string{"players", "bids", "sealed_bids"} {
		var n int
		if err := f.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatalf("counting %s: %v", table, err)
		}
		out[i] = n
	}
	return out
}
