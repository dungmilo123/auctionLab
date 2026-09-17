package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"auctionlab/internal/game"
	"auctionlab/internal/hub"
	"auctionlab/internal/store"
	"auctionlab/internal/tunnel"
)

// This file owns the boot's wiring contracts: what the optional tunnel flags
// decide, that PUBLIC_URL is resolved with the game package's own rule and never
// echoed, that the status callback reaches BOTH consumers through D-123's
// one-way wire, and that the production heartbeat owner really beats.
//
// Every fixture here builds the same graph run() builds (store → engine → hub →
// publisher), because the contracts under test are about that graph and not
// about a mock of it.

// TestNewServerTimeouts pins the slowloris hardening (WR-03 / gosec G112) at
// the constructor the binary actually uses: header reads and idle keep-alives
// are bounded, while ReadTimeout/WriteTimeout stay unset because they span the
// whole connection — which would terminate the long-lived /ws upgrade.
func TestNewServerTimeouts(t *testing.T) {
	srv := newServer(":0", http.NotFoundHandler())

	if srv.ReadHeaderTimeout <= 0 {
		t.Error("ReadHeaderTimeout is unset; a dribbling client can hold a connection slot indefinitely")
	}
	if srv.IdleTimeout <= 0 {
		t.Error("IdleTimeout is unset; an idle keep-alive connection is never reclaimed")
	}
	if srv.ReadTimeout != 0 {
		t.Errorf("ReadTimeout = %v, want 0 — it covers the whole connection and would cut off /ws", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 — it covers the whole connection and would cut off /ws", srv.WriteTimeout)
	}
	if srv.Addr != ":0" || srv.Handler == nil {
		t.Errorf("newServer(:0, h) = %+v, want the addr and handler it was given", srv)
	}
}

// --- fixtures ---------------------------------------------------------------

// testAdminKey is the production SHAPE (64 hex characters, D-74) but no real
// key: a fixture that could match a live server's capability would be a leak,
// not a test.
const testAdminKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// adminJoinJSON is the admin handshake frame with the capability on D-76's
// script channel.
const adminJoinJSON = `{"type":"join","role":"admin","key":"` + testAdminKey + `"}`

// newServerEnv builds the engine and hub exactly as run() does, on a temp
// database, so the wiring can be asserted without a listener.
func newServerEnv(t *testing.T) (*game.Engine, *hub.Hub) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "auction.db"))
	if err != nil {
		t.Fatalf("opening the test database: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
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

	ws := hub.New(engine, testAdminKey)
	engine.SetPublisher(ws.Fanout)
	return engine, ws
}

// newLivenessEnv is newServerEnv with a heartbeat cadence a test can observe:
// the production owner is exercised, its 25-second production period is not.
func newLivenessEnv(t *testing.T) (*game.Engine, *hub.Hub) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "auction.db"))
	if err != nil {
		t.Fatalf("opening the test database: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
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

	policy := hub.DefaultPolicy
	policy.HeartbeatPeriod = 10 * time.Millisecond
	ws := hub.NewWithPolicy(engine, policy, testAdminKey)
	engine.SetPublisher(ws.Fanout)
	return engine, ws
}

// serverFrame is one decoded server frame, as far as these cases read it.
type serverFrame struct {
	Type string          `json:"type"`
	Seq  int64           `json:"seq"`
	Rev  int64           `json:"rev"`
	Data json.RawMessage `json:"data"`
}

// dialWS serves the hub on a real listener and dials it, mirroring the hub
// package's own fixture: the wait group keeps a connection's deferred teardown
// (which may commit presence) from running after the store has closed.
func dialWS(t *testing.T, ws *hub.Hub) *websocket.Conn {
	t.Helper()

	var handlers sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Done()
		ws.ServeWS(w, r)
	}))
	t.Cleanup(func() {
		srv.Close()
		handlers.Wait()
	})

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
		}
		t.Fatalf("dialing %s: %v (HTTP %d)", url, err, status)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// readServerFrame reads the next frame and fails the case if none arrives.
func readServerFrame(t *testing.T, conn *websocket.Conn) serverFrame {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading a frame: %v", err)
	}
	var env serverFrame
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("frame is not a JSON object: %v (%s)", err, raw)
	}
	return env
}

// readServerType reads until the named frame type arrives.
func readServerType(t *testing.T, conn *websocket.Conn, typ string) serverFrame {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no %q frame arrived within the read budget", typ)
		}
		env := readServerFrame(t, conn)
		if env.Type == typ {
			return env
		}
	}
}

// serverData is one frame's data object.
func serverData(t *testing.T, env serverFrame) map[string]json.RawMessage {
	t.Helper()

	var data map[string]json.RawMessage
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("data is not a JSON object: %v (%s)", err, env.Data)
	}
	return data
}

// --- the flag decisions -----------------------------------------------------

// TestTunnelPlan pins D-124/D-129's decision table: what `--tunnel` starts, what
// it logs, and that NOTHING in it can fail the boot. A missing binary, a proxy in
// the environment and a conflicting cloudflared config are all operator-facing
// lines plus a still-running auction.
func TestTunnelPlan(t *testing.T) {
	notFound := func(string) (string, error) { return "", errors.New("executable file not found in $PATH") }
	found := func(path string) func(string) (string, error) {
		return func(string) (string, error) { return path, nil }
	}
	env := func(kv map[string]string) func(string) string {
		return func(key string) string { return kv[key] }
	}
	joined := func(plan tunnelPlan) string { return strings.Join(plan.Log, "\n") }

	t.Run("without --tunnel nothing is started and nothing is logged", func(t *testing.T) {
		plan := planTunnel(false, "/usr/local/bin/cloudflared", env(nil), found("/usr/local/bin/cloudflared"), t.TempDir())
		if plan.Start || plan.Bin != "" || len(plan.Log) != 0 {
			t.Errorf("planTunnel(disabled) = %+v, want an empty plan", plan)
		}
	})

	t.Run("a missing binary warns with the fix instead of failing the boot", func(t *testing.T) {
		plan := planTunnel(true, "", env(nil), notFound, t.TempDir())
		if plan.Start {
			t.Error("a tunnel was started without a binary")
		}
		log := joined(plan)
		for _, want := range []string{"cloudflared", "TUNNEL_BIN", "install-cloudflared.sh", "LAN"} {
			if !strings.Contains(log, want) {
				t.Errorf("the warning does not mention %q: %s", want, log)
			}
		}
	})

	t.Run("the flag wins TUNNEL_BIN, which wins PATH", func(t *testing.T) {
		cases := []struct {
			name string
			flag string
			env  map[string]string
			path string
			want string
		}{
			{"the flag path", "/flag/cloudflared", map[string]string{"TUNNEL_BIN": "/env/cloudflared"}, "/path/cloudflared", "/flag/cloudflared"},
			{"TUNNEL_BIN", "", map[string]string{"TUNNEL_BIN": "/env/cloudflared"}, "/path/cloudflared", "/env/cloudflared"},
			{"PATH", "", nil, "/path/cloudflared", "/path/cloudflared"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				plan := planTunnel(true, tc.flag, env(tc.env), found(tc.path), t.TempDir())
				if !plan.Start {
					t.Fatal("no tunnel was started although a binary was resolvable")
				}
				if plan.Bin != tc.want {
					t.Errorf("bin = %q, want %q", plan.Bin, tc.want)
				}
				if !strings.Contains(joined(plan), tc.want) {
					t.Errorf("the boot log does not name the resolved binary path:\n%s", joined(plan))
				}
			})
		}
	})

	t.Run("proxy variables are reported by NAME and never by value", func(t *testing.T) {
		secret := "http://user:pass@proxy.internal:3128"
		plan := planTunnel(true, "/usr/local/bin/cloudflared", env(map[string]string{
			"HTTPS_PROXY": secret,
		}), found("/usr/local/bin/cloudflared"), t.TempDir())
		log := joined(plan)
		if !strings.Contains(log, "HTTPS_PROXY") {
			t.Errorf("the proxy variable is not named in the boot log:\n%s", log)
		}
		if strings.Contains(log, secret) || strings.Contains(log, "proxy.internal") {
			t.Errorf("the boot log echoed the proxy value:\n%s", log)
		}
	})

	t.Run("an unknown TUNNEL_PROTOCOL is dropped with a warning that names the variable", func(t *testing.T) {
		plan := planTunnel(true, "/usr/local/bin/cloudflared", env(map[string]string{
			"TUNNEL_PROTOCOL": "htpp2",
		}), found("/usr/local/bin/cloudflared"), t.TempDir())
		if !plan.Start {
			t.Fatal("a bad protocol must warn, never refuse to start")
		}
		log := joined(plan)
		for _, want := range []string{"TUNNEL_PROTOCOL", "quic", "http2"} {
			if !strings.Contains(log, want) {
				t.Errorf("the warning does not mention %q:\n%s", want, log)
			}
		}
		if strings.Contains(log, "htpp2") {
			t.Errorf("the boot log echoed the value:\n%s", log)
		}

		// The two known literals are the documented escape hatch and stay silent.
		for _, ok := range []string{"quic", "http2"} {
			plan := planTunnel(true, "/usr/local/bin/cloudflared", env(map[string]string{
				"TUNNEL_PROTOCOL": ok,
			}), found("/usr/local/bin/cloudflared"), t.TempDir())
			if strings.Contains(joined(plan), "TUNNEL_PROTOCOL") {
				t.Errorf("a valid TUNNEL_PROTOCOL=%s was warned about:\n%s", ok, joined(plan))
			}
		}
	})

	t.Run("a cloudflared config file is reported with its path", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".cloudflared")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("creating the fake config directory: %v", err)
		}
		conflict := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(conflict, []byte("tunnel: some-named-tunnel\n"), 0o644); err != nil {
			t.Fatalf("writing the fake config file: %v", err)
		}

		plan := planTunnel(true, "/usr/local/bin/cloudflared", env(nil), found("/usr/local/bin/cloudflared"), home)
		if !plan.Start {
			t.Fatal("the config conflict must warn, never refuse to start")
		}
		if log := joined(plan); !strings.Contains(log, conflict) {
			t.Errorf("the conflict warning does not name the measured path %q:\n%s", conflict, log)
		}
	})
}

// TestTunnelPublicURL pins D-127's override resolution and D-129's non-fatal
// rule: the flag beats PUBLIC_URL, an unusable value is ignored rather than
// advertised, and the VALUE never reaches the operator-facing line — a public
// URL may carry a query string, the same rule that keeps ADMIN_KEY out of logs.
func TestTunnelPublicURL(t *testing.T) {
	env := func(kv map[string]string) func(string) string {
		return func(key string) string { return kv[key] }
	}
	const good = "https://demo.example/join"

	t.Run("the flag wins PUBLIC_URL", func(t *testing.T) {
		value, source, warning := resolvePublicURL(good, env(map[string]string{"PUBLIC_URL": "https://env.example/"}))
		if value != good || warning != "" {
			t.Errorf("resolvePublicURL = (%q, %q), want (%q, no warning)", value, warning, good)
		}
		if !strings.Contains(source, "public-url") {
			t.Errorf("source = %q, want it to name the flag", source)
		}
	})

	t.Run("PUBLIC_URL is used when the flag is empty", func(t *testing.T) {
		value, source, warning := resolvePublicURL("", env(map[string]string{"PUBLIC_URL": good}))
		if value != good || warning != "" {
			t.Errorf("resolvePublicURL = (%q, %q), want (%q, no warning)", value, warning, good)
		}
		if !strings.Contains(source, "PUBLIC_URL") {
			t.Errorf("source = %q, want it to name the environment variable", source)
		}
	})

	t.Run("no override at all is silent", func(t *testing.T) {
		value, source, warning := resolvePublicURL("", env(nil))
		if value != "" || source != "" || warning != "" {
			t.Errorf("resolvePublicURL(none) = (%q, %q, %q), want an empty, silent result", value, source, warning)
		}
	})

	t.Run("an unusable override warns without echoing the value", func(t *testing.T) {
		raw := "ftp://user:secret@example.internal/room?code=hidden"
		value, _, warning := resolvePublicURL(raw, env(nil))
		if value != "" {
			t.Errorf("value = %q, want the unusable override dropped (the engine must never advertise it)", value)
		}
		if warning == "" {
			t.Fatal("no warning for an unusable override")
		}
		for _, leak := range []string{raw, "hidden", "secret"} {
			if strings.Contains(warning, leak) {
				t.Errorf("the warning echoed %q: %s", leak, warning)
			}
		}
	})

	t.Run("an unusable PUBLIC_URL is ignored the same way", func(t *testing.T) {
		value, _, warning := resolvePublicURL("", env(map[string]string{"PUBLIC_URL": "not-a-url"}))
		if value != "" || warning == "" {
			t.Errorf("resolvePublicURL(bad env) = (%q, %q), want it dropped with a warning", value, warning)
		}
	})
}

// --- the status callback ----------------------------------------------------

// TestTunnelStatusSink pins D-123's one-way wire end to end: one published
// tunnel status moves the engine's runtime URL (so the QR rotates) AND reaches
// the admin console as the control frame — through the same call, with no
// revision bump and no player ever seeing the status.
func TestTunnelStatusSink(t *testing.T) {
	const url = "https://sink-test.trycloudflare.com"

	engine, ws := newServerEnv(t)

	admin := dialWS(t, ws)
	writeServerFrame(t, admin, adminJoinJSON)
	readServerType(t, admin, "hello")
	readServerType(t, admin, "state.snapshot")

	player := dialWS(t, ws)
	writeServerFrame(t, player, `{"type":"join","role":"player","nickname":"An"}`)
	readServerType(t, player, "hello")
	readServerType(t, player, "state.snapshot")

	// The player's join is a mutation: it fans one snapshot to every joined
	// connection, including the admin.
	readServerType(t, admin, "state.snapshot")
	readServerType(t, player, "state.snapshot")

	rev := engine.Rev()
	tunnelStatusSink(ws, engine)(tunnel.Status{URL: url, Healthy: true})

	// The engine half: the effective join URL moved, at the current revision.
	screen, ok := engine.Snapshot(game.RoleScreen, "").(game.ScreenView)
	if !ok {
		t.Fatalf("screen snapshot has type %T", engine.Snapshot(game.RoleScreen, ""))
	}
	if screen.JoinURL != url {
		t.Errorf("screen join_url = %q, want the published tunnel URL %q", screen.JoinURL, url)
	}
	adminView, ok := engine.Snapshot(game.RoleAdmin, "").(game.AdminView)
	if !ok {
		t.Fatalf("admin snapshot has type %T", engine.Snapshot(game.RoleAdmin, ""))
	}
	if adminView.SelectedJoinURL != url || adminView.JoinURLSource != "tunnel" {
		t.Errorf("admin selection = %q (%q), want %q (%q)", adminView.SelectedJoinURL, adminView.JoinURLSource, url, "tunnel")
	}
	if got := engine.Rev(); got != rev {
		t.Errorf("revision = %d, want the unchanged %d: a tunnel fact is not a game mutation (D-35)", got, rev)
	}

	// The hub half: the admin console receives the control frame...
	control := readServerType(t, admin, "tunnel.status")
	if control.Rev != rev {
		t.Errorf("tunnel.status rev = %d, want %d", control.Rev, rev)
	}
	var payload struct {
		URL     string `json:"url"`
		Healthy bool   `json:"healthy"`
	}
	if err := json.Unmarshal(control.Data, &payload); err != nil {
		t.Fatalf("tunnel.status data: %v (%s)", err, control.Data)
	}
	if payload.URL != url || !payload.Healthy {
		t.Errorf("tunnel.status payload = %+v, want {%q true}", payload, url)
	}

	// ...and the player's next frame is the runtime snapshot, never the status.
	next := readServerFrame(t, player)
	if next.Type == "tunnel.status" {
		t.Error("the player received tunnel.status: the control frame is admin/screen only (T-07-05)")
	}
}

// --- the liveness owner -----------------------------------------------------

// TestServerHeartbeatLifecycle pins the production owner of D-132's beat: the
// same startLiveness run() defers starts the heartbeat (and the ticker) after the
// hub exists, a QUIET joined socket receives the control frame without any
// revision change, and stopping the owner really stops the beats.
func TestServerHeartbeatLifecycle(t *testing.T) {
	engine, ws := newLivenessEnv(t)

	conn := dialWS(t, ws)
	writeServerFrame(t, conn, `{"type":"join","role":"screen"}`)
	readServerType(t, conn, "hello")
	readServerType(t, conn, "state.snapshot")

	rev := engine.Rev()

	stop := startLiveness(ws)
	t.Cleanup(stop)

	beat := readServerType(t, conn, "heartbeat")
	if beat.Rev != rev {
		t.Errorf("heartbeat rev = %d, want the unchanged %d (a beat is not a mutation, D-132)", beat.Rev, rev)
	}
	if data := serverData(t, beat); len(data) != 1 {
		t.Errorf("heartbeat data keys = %v, want exactly [ts]", data)
	}
	if got := engine.Rev(); got != rev {
		t.Errorf("engine revision after a beat = %d, want %d", got, rev)
	}

	stop() // the deferred stop, called early: idempotent by contract

	// At most one beat can be in flight when stop returns. Let it land, then
	// require silence: the read below ends on its deadline, which is why it is
	// the LAST thing this case does with the socket (gorilla's read deadline is
	// terminal).
	time.Sleep(30 * time.Millisecond)
	var strays []string
	_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break // the deadline: the silence is the assertion
		}
		var env serverFrame
		if err := json.Unmarshal(raw, &env); err == nil {
			strays = append(strays, env.Type)
		}
	}
	if len(strays) > 1 {
		t.Errorf("frames after the liveness owner stopped = %v, want at most the single beat already in flight", strays)
	}
	for _, typ := range strays {
		if typ != "heartbeat" {
			t.Errorf("a %q frame arrived in a quiet lobby: only the heartbeat may speak there", typ)
		}
	}
}

// writeServerFrame sends one text frame and fails the case if the socket refuses
// it.
func writeServerFrame(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()

	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatalf("writing %s: %v", payload, err)
	}
}
