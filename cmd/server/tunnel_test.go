package main

import (
	"auctionlab/internal/testpg"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
	"auctionlab/internal/hub"
	"auctionlab/internal/store"
	"auctionlab/internal/tunnel"
)

// This file owns D-135/D-136/D-137's acceptance layer: the tunnel-death drill
// and the shape of the shell/Make targets that run the 20-bot fleet.
//
// The drill drives the REAL production wire — tunnel.Manager → tunnelStatusSink
// → hub broadcast + engine runtime URL — with only two seams: the stub binary
// (D-124's TUNNEL_BIN, so no cloudflared and no network are involved) and the
// injected Probe (D-126), which points at this process's own hub instead of a
// trycloudflare hostname. Nothing else is faked: the child is a real process
// that is really SIGKILLed, and the fallback the drill asserts is the engine's
// own resolution (D-127), not a mock of it.

// drillBudget bounds every wait in this file. A `/bin/sh` spawn plus a probe
// round-trip against an in-process listener sits well inside it, and a broken
// wiring fails the case instead of hanging the package.
const drillBudget = 5 * time.Second

// drillLAN is the LAN candidate the drill offers the engine. The fallback rung
// is observable only because a candidate exists: with none, an unhealthy tunnel
// resolves to the empty string and "fell back" would be indistinguishable from
// "never had anything" (D-129).
const drillLAN = "http://192.168.1.50:8080/"

// --- the hub host -----------------------------------------------------------

// drillEnv is the graph run() builds — store → engine → hub → publisher — plus
// ONE httptest listener whose URL is stable for the whole case. The tunnel
// probe dials it, and so does every socket the drill opens: the manager's
// restart loop must reach a live hub without the drill re-registering a
// listener on every probe.
type drillEnv struct {
	engine *game.Engine
	hub    *hub.Hub
	srv    *httptest.Server

	handlers sync.WaitGroup
	mu       sync.Mutex
	conns    []*websocket.Conn
}

// newDrillEnv builds the production graph on a temp database with the drill's
// LAN candidate, then serves the hub on a real listener.
func newDrillEnv(t *testing.T, candidates ...string) *drillEnv {
	t.Helper()

	st, err := store.Open(testpg.URL(t, filepath.Join(t.TempDir(), "auction.db")))
	if err != nil {
		t.Fatalf("opening the drill database: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("applying the schema: %v", err)
	}

	engine, err := game.New(game.Config{
		Store:             st,
		Now:               time.Now,
		JoinURLCandidates: candidates,
		Fatal:             func(err error) { t.Errorf("engine reported an unrecoverable commit failure: %v", err) },
	})
	if err != nil {
		t.Fatalf("building the engine: %v", err)
	}

	env := &drillEnv{engine: engine, hub: hub.New(engine, testAdminKey)}
	engine.SetPublisher(env.hub.Fanout)

	env.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env.handlers.Add(1)
		defer env.handlers.Done()
		env.hub.ServeWS(w, r)
	}))
	t.Cleanup(func() {
		env.closeConns()
		env.srv.Close()
		env.handlers.Wait()
	})
	return env
}

// wsURL is the hub's dialable address.
func (e *drillEnv) wsURL() string {
	return "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws"
}

// dial opens one socket against the shared listener and remembers it so the
// teardown can close it before the store does.
func (e *drillEnv) dial(t *testing.T) *websocket.Conn {
	t.Helper()

	conn, resp, err := websocket.DefaultDialer.Dial(e.wsURL(), nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
		}
		t.Fatalf("dialing %s: %v (HTTP %d)", e.wsURL(), err, status)
	}
	e.mu.Lock()
	e.conns = append(e.conns, conn)
	e.mu.Unlock()
	return conn
}

// closeConns closes every socket the drill opened.
func (e *drillEnv) closeConns() {
	e.mu.Lock()
	conns := append([]*websocket.Conn(nil), e.conns...)
	e.conns = nil
	e.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// probe is the injected D-126 health check: the real screen handshake (join →
// hello seq 1 → state.snapshot seq 2), performed against this process's own
// hub. It never looks at the tunnel URL, which is exactly the seam D-137 needs
// to flip `healthy` without a network.
func (e *drillEnv) probe(ctx context.Context, _ string) error {
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, e.wsURL(), nil)
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return fmt.Errorf("dialing the hub: %w", err)
	}
	defer func() { _ = conn.Close() }()

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Second)
	}
	_ = conn.SetWriteDeadline(deadline)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"join","role":"screen"}`)); err != nil {
		return fmt.Errorf("joining as screen: %w", err)
	}

	for _, want := range []string{"hello", "state.snapshot"} {
		_ = conn.SetReadDeadline(deadline)
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("reading %s: %w", want, err)
		}
		var env serverFrame
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decoding a frame: %w", err)
		}
		if env.Type != want {
			return fmt.Errorf("handshake frame = %q, want %q", env.Type, want)
		}
	}
	return nil
}

// screenJoinURL is the effective join URL the projector would render, polled
// until it settles: the publish that moves it runs on the manager's goroutine,
// so the drill waits for the projection instead of racing it.
func (e *drillEnv) screenJoinURL(t *testing.T, want string) {
	t.Helper()

	deadline := time.Now().Add(drillBudget)
	var got string
	for {
		view, ok := e.engine.Snapshot(game.RoleScreen, "").(game.ScreenView)
		if !ok {
			t.Fatalf("screen snapshot has type %T", e.engine.Snapshot(game.RoleScreen, ""))
		}
		got = view.JoinURL
		if got == want || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got != want {
		t.Fatalf("screen join_url = %q, want %q", got, want)
	}
}

// adminURL polls the admin projection's selection and its source rung.
func (e *drillEnv) adminURL(t *testing.T, wantURL, wantSource string) {
	t.Helper()

	deadline := time.Now().Add(drillBudget)
	var gotURL, gotSource string
	for {
		view, ok := e.engine.Snapshot(game.RoleAdmin, "").(game.AdminView)
		if !ok {
			t.Fatalf("admin snapshot has type %T", e.engine.Snapshot(game.RoleAdmin, ""))
		}
		gotURL, gotSource = view.SelectedJoinURL, view.JoinURLSource
		if (gotURL == wantURL && gotSource == wantSource) || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if gotURL != wantURL || gotSource != wantSource {
		t.Fatalf("admin selection = %q (%q), want %q (%q)", gotURL, gotSource, wantURL, wantSource)
	}
}

// rev polls the engine's revision until want is reached. Player joins are the
// drill's mutations, and their commit is observable a moment after the joiner's
// own handshake lands.
func (e *drillEnv) rev(t *testing.T, want int64) int64 {
	t.Helper()

	deadline := time.Now().Add(drillBudget)
	for {
		got := e.engine.Rev()
		if got >= want || time.Now().After(deadline) {
			if got < want {
				t.Fatalf("engine revision = %d, want at least %d: the mutation did not settle", got, want)
			}
			return got
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// joinPlayer joins one player through the real handshake and returns its
// socket. A player join is a committed mutation (unlike admin/screen), which is
// what makes it the drill's "the auction keeps running" probe.
func (e *drillEnv) joinPlayer(t *testing.T, nickname string) *websocket.Conn {
	t.Helper()

	conn := e.dial(t)
	writeServerFrame(t, conn, fmt.Sprintf(`{"type":"join","role":"player","nickname":%q}`, nickname))
	readServerType(t, conn, "hello")
	readServerType(t, conn, "state.snapshot")
	return conn
}

// --- the status recorder ----------------------------------------------------

// drillStatus is every tunnel status the manager published, in order, plus a
// channel for the waits. The drill wraps the production sink with it, so what
// it records is exactly what the hub and the engine were handed.
type drillStatus struct {
	mu  sync.Mutex
	all []tunnel.Status
	ch  chan tunnel.Status
}

func newDrillStatus() *drillStatus {
	return &drillStatus{ch: make(chan tunnel.Status, 256)}
}

func (r *drillStatus) publish(s tunnel.Status) {
	r.mu.Lock()
	r.all = append(r.all, s)
	r.mu.Unlock()
	select {
	case r.ch <- s:
	default: // the snapshot below still carries it
	}
}

func (r *drillStatus) snapshot() []tunnel.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]tunnel.Status(nil), r.all...)
}

// urls is the distinct non-empty URLs published so far, in first order.
func (r *drillStatus) urls() []string {
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

func (r *drillStatus) await(t *testing.T, want func(tunnel.Status) bool) tunnel.Status {
	t.Helper()

	deadline := time.After(drillBudget)
	for {
		select {
		case s := <-r.ch:
			if want(s) {
				return s
			}
		case <-deadline:
			t.Fatalf("no matching tunnel status within %s; published: %v", drillBudget, r.snapshot())
			return tunnel.Status{}
		}
	}
}

// awaitNewURL waits for a non-empty URL that has not been published before:
// that is the rotation the restarted stub proves.
func (r *drillStatus) awaitNewURL(t *testing.T) tunnel.Status {
	t.Helper()

	seen := make(map[string]bool)
	for _, s := range r.snapshot() {
		if s.URL != "" {
			seen[s.URL] = true
		}
	}
	return r.await(t, func(s tunnel.Status) bool { return s.URL != "" && !seen[s.URL] })
}

// assertQuiet fails the case when a new URL appears within d — how "a
// deliberate Close never schedules a successor" is asserted without racing the
// manager's own shutdown.
func (r *drillStatus) assertQuiet(t *testing.T, d time.Duration) {
	t.Helper()

	before := len(r.urls())
	time.Sleep(d)
	if after := len(r.urls()); after != before {
		t.Fatalf("a URL was published after the manager closed: %v", r.urls())
	}
}

// --- the diagnostics recorder -----------------------------------------------

// drillLogs captures the manager's diagnostics. The text is asserted because
// D-125/T-07-17 make "a restart was reported, with its reason and delay" part
// of the evidence, not decoration.
type drillLogs struct {
	mu   sync.Mutex
	text strings.Builder
}

func (l *drillLogs) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(&l.text, format+"\n", args...)
}

func (l *drillLogs) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.text.String()
}

func (l *drillLogs) contains(substr string) bool { return strings.Contains(l.all(), substr) }

// --- the stub ---------------------------------------------------------------

// stubTunnel writes the D-124 stub binary: each invocation announces a URL that
// carries its own generation number (that is what proves rotation), records its
// pid so the drill can really SIGKILL it, and then becomes a long sleep so the
// process the manager supervises is the one the drill kills.
func stubTunnel(t *testing.T, dir, generationFile, pidFile string) string {
	t.Helper()

	path := filepath.Join(dir, "cloudflared-stub")
	body := fmt.Sprintf(`
n=0
[ -f %[1]q ] && n=$(cat %[1]q)
n=$((n+1))
printf '%%s' "$n" > %[1]q
printf '%%s' "$$" > %[2]q
echo "ERR |  https://stub-$n.trycloudflare.com  |" >&2
exec sleep 3600
`, generationFile, pidFile)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("writing the stub binary: %v", err)
	}
	return path
}

// stubPID is the pid the live stub generation recorded.
func stubPID(t *testing.T, pidFile string) int {
	t.Helper()

	deadline := time.Now().Add(drillBudget)
	for {
		raw, err := os.ReadFile(pidFile)
		if err == nil && strings.TrimSpace(string(raw)) != "" {
			var pid int
			if _, err := fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &pid); err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the stub never recorded its pid in %s: %v", pidFile, err)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// --- TestTunnelDeathDrill ---------------------------------------------------

// TestTunnelDeathDrill is D-137's desk equivalent of the venue drill (REH-01):
// a stub cloudflared is killed mid-game and the drill asserts, in order, that
// the declared no-tunnel state is published, the QR falls back to the LAN
// candidate, play keeps settling locally, the supervisor restarts with a
// ROTATED url, the probe flips it healthy again, and a deliberate Close never
// schedules a successor. The whole path runs through the production sink, so
// the hub frame and the engine projection are asserted together.
func TestTunnelDeathDrill(t *testing.T) {
	env := newDrillEnv(t, drillLAN)

	dir := t.TempDir()
	bin := stubTunnel(t, dir, filepath.Join(dir, "generation"), filepath.Join(dir, "pid"))

	rec := newDrillStatus()
	logs := &drillLogs{}

	mgr := tunnel.New(tunnel.Config{
		Port:         8080,
		Bin:          bin,
		Home:         t.TempDir(), // no config.yaml: the conflict scan must find nothing
		Publish:      func(s tunnel.Status) { rec.publish(s); tunnelStatusSink(env.hub, env.engine)(s) },
		Probe:        env.probe,
		Logf:         logs.logf,
		BackoffBase:  20 * time.Millisecond,
		BackoffCap:   40 * time.Millisecond,
		StableAfter:  time.Hour, // never stable: every death restarts on the short schedule
		ProbeEvery:   25 * time.Millisecond,
		ProbeTimeout: time.Second,
		StopWait:     time.Second,
		Jitter:       func(d time.Duration) time.Duration { return d },
	})
	mgr.Start()
	t.Cleanup(mgr.Close)

	// The admin console is the drill's witness on the wire: D-123's control
	// frame is what tells a human the tunnel moved.
	admin := env.dial(t)
	writeServerFrame(t, admin, adminJoinJSON)
	readServerType(t, admin, "hello")
	readServerType(t, admin, "state.snapshot")

	// 1. The announcement is published UNHEALTHY — health is probe-derived.
	first := rec.await(t, func(s tunnel.Status) bool { return strings.HasPrefix(s.URL, "https://stub-") })
	if first.Healthy {
		t.Fatalf("the announcement was published healthy: %+v", first)
	}
	revBeforeTunnel := env.engine.Rev()

	// 2. The probe's verdict flips it, and the production sink moves BOTH
	// consumers: the admin console sees the control frame, the engine's
	// projection adopts the tunnel URL without moving the revision (D-35).
	rec.await(t, func(s tunnel.Status) bool { return s.URL == first.URL && s.Healthy })
	control := readTunnelStatus(t, admin, true)
	if control != first.URL {
		t.Fatalf("the admin console was told %q, want %q", control, first.URL)
	}
	env.screenJoinURL(t, first.URL)
	env.adminURL(t, first.URL, "tunnel")
	if got := env.engine.Rev(); got != revBeforeTunnel {
		t.Errorf("revision = %d after a tunnel fact, want the unchanged %d (D-35)", got, revBeforeTunnel)
	}

	// 3. A local mutation settles while the tunnel is alive: the drill's
	// baseline for "the auction kept running".
	base := env.engine.Rev()
	env.joinPlayer(t, "Lan-A")
	alive := env.rev(t, base+1)
	readServerType(t, admin, "state.snapshot")

	// 4. Kill the stub mid-game. This is a real SIGKILL of a real process.
	pid := stubPID(t, filepath.Join(dir, "pid"))
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("killing the stub (pid %d): %v", pid, err)
	}

	// 5. The URL dies with the child: the declared no-tunnel state is
	// published and the QR falls back to the LAN candidate (D-128/D-129).
	dead := rec.await(t, func(s tunnel.Status) bool { return s.URL == "" && !s.Healthy })
	_ = dead
	env.screenJoinURL(t, drillLAN)
	env.adminURL(t, drillLAN, "candidate")

	// 6. Play still settles locally with the tunnel gone (D-137's criterion 2).
	env.joinPlayer(t, "Lan-B")
	readServerType(t, admin, "state.snapshot")
	settled := env.rev(t, alive+1)
	if settled <= alive {
		t.Fatalf("revision = %d after the tunnel died, want at least %d: the local mutation did not settle", settled, alive+1)
	}

	// 7. The supervisor restarts with a ROTATED url, and the probe flips the
	// new generation healthy — the QR update the venue drill depends on.
	second := rec.awaitNewURL(t)
	if second.URL == first.URL {
		t.Fatalf("the successor announced the same URL %q: rotation is what proves a new generation", second.URL)
	}
	if second.Healthy {
		t.Fatalf("a freshly announced URL must not be healthy before its probe: %+v", second)
	}
	rec.await(t, func(s tunnel.Status) bool { return s.URL == second.URL && s.Healthy })
	env.screenJoinURL(t, second.URL)
	env.adminURL(t, second.URL, "tunnel")
	if got := readTunnelStatus(t, admin, true); got != second.URL {
		t.Fatalf("the admin console never saw the rotated URL: got %q, want %q", got, second.URL)
	}

	// 8. A deliberate Close ends the loop for good.
	mgr.Close()
	rec.assertQuiet(t, 200*time.Millisecond)

	// The restart was reported with its reason and delay: the operator-facing
	// half of D-125/T-07-17.
	if log := logs.all(); !strings.Contains(log, "restarting in") {
		t.Errorf("the supervisor never reported the restart:\n%s", log)
	}
}

// readTunnelStatus waits for the next tunnel.status frame on conn whose
// `healthy` matches, and returns its URL. Frames of other types (the state
// fan-outs the drill's joins cause) are skipped, which is why it cannot use
// readServerType.
func readTunnelStatus(t *testing.T, conn *websocket.Conn, healthy bool) string {
	t.Helper()

	deadline := time.Now().Add(drillBudget)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no tunnel.status frame with healthy=%t arrived within %s", healthy, drillBudget)
		}
		_ = conn.SetReadDeadline(deadline)
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("reading a tunnel.status frame: %v", err)
		}
		var env serverFrame
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("frame is not a JSON object: %v (%s)", err, raw)
		}
		if env.Type != "tunnel.status" {
			continue
		}
		var payload struct {
			URL     string `json:"url"`
			Healthy bool   `json:"healthy"`
		}
		if err := json.Unmarshal(env.Data, &payload); err != nil {
			t.Fatalf("tunnel.status data: %v (%s)", err, env.Data)
		}
		if payload.Healthy == healthy {
			return payload.URL
		}
	}
}

// --- TestAcceptanceHarnessShape ---------------------------------------------

// TestAcceptanceHarnessShape pins the phase gate's shell surface: D-135's
// acceptance script must create its own database, pin its own admin key, wait
// for health, reap its child on every exit path and hand the harness's exit
// code through unchanged — and the Makefile must grow the three new targets
// without disturbing `loadtest` or `test-race`. A script that leaks a server or
// a dev database is the failure this case exists to prevent.
func TestAcceptanceHarnessShape(t *testing.T) {
	root := filepath.Join("..", "..")

	scriptPath := filepath.Join(root, "scripts", "loadtest-acceptance.sh")
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("D-135's acceptance script is missing: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s is not executable (mode %s)", scriptPath, info.Mode().Perm())
	}

	script, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("reading %s: %v", scriptPath, err)
	}
	for _, want := range []struct{ what, substr string }{
		{"explicit PostgreSQL test configuration", "TEST_DATABASE_URL"},
		{"owned disposable schema creation and cleanup", "./scripts/dbfixture"},
		{"a temporary directory", "mktemp"},
		{"a pinned admin capability", "ADMIN_KEY"},
		{"child cleanup on every exit path", "trap"},
		{"a bounded health wait", "/api/health"},
		{"the harness's exit code passed through", "exit"},
	} {
		if !strings.Contains(string(script), want.substr) {
			t.Errorf("the acceptance script has no %s (%q):\n%s", want.what, want.substr, script)
		}
	}

	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}
	mk := string(makefile)

	for _, want := range []struct{ what, substr string }{
		{"the phony declaration", "loadtest-20 loadtest-race loadtest-tunnel"},
		{"the two-bot smoke target on an owned disposable schema", "loadtest:\n\t$(GO) build -o bin/auctionlab-loadtest ./cmd/server\n\tLOADTEST_PORT=$(LOADTEST_PORT) ./scripts/loadtest-acceptance.sh ./bin/auctionlab-loadtest"},
		{"the race test target, unchanged", "test-race:\n\t$(GO) test -race -timeout 120s ./..."},
		{"a race-built server binary", "-o bin/auctionlab-race"},
		{"the race flag on the server build", "build -race -o bin/auctionlab-race"},
		{"the tunnel drill target", "loadtest-tunnel:"},
		{"the drill's own command", "run TestTunnelDeathDrill"},
	} {
		if !strings.Contains(mk, want.substr) {
			t.Errorf("the Makefile has no %s (%q):\n%s", want.what, want.substr, mk)
		}
	}
}
