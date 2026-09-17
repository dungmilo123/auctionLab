package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/gorilla/websocket"
)

// This file owns the health primitive D-126 requires: a REAL WebSocket
// round-trip through the public URL. Process liveness, `--metrics`, DNS and an
// HTTP GET of /api/health all prove something weaker than the upgrade path, and
// the #1652 failure mode (a tunnel that drops the `Upgrade` header) looks
// perfectly healthy to every one of them. Only a completed handshake proves the
// venue's phones can reach the server.

// The wire literals the probe asserts. They are written as literals on purpose
// — the same choice scripts/loadtest/main.go makes — so this package stays a
// leaf that internal/hub never has to import (D-123's "no game, hub or store
// package imports internal/tunnel" is the important direction, and a literal
// keeps the reverse direction empty too). The handshake itself is frozen by
// D-23, and a rename in the hub must fail loudly: the probe would stop
// reporting healthy, which is the safe direction for a health check.
const (
	probeHelloType    = "hello"
	probeSnapshotType = "state.snapshot"
	probeErrorType    = "error"
	probeScreenRole   = "screen"
)

// probeJoin is the exact frame a screen client sends: no token, no nickname,
// no admin key. Screen is the role the projector uses, and it is deliberately
// NOT the player role the retired `ws-probe.mjs` used — a player join creates a
// player row, and a probe that ran every 30 seconds would fill the database
// with players nobody ever saw (D-126).
const probeJoin = `{"type":"join","role":"screen"}`

// ProbeFunc is one probe attempt: dial the tunnel origin, complete the screen
// handshake, close. nil means healthy. It is injectable because the manager's
// tests must be able to flip health without an Internet-facing URL — the stub
// binary prints `https://stub-N.trycloudflare.com`, which by construction does
// not resolve (D-137).
type ProbeFunc func(ctx context.Context, tunnelURL string) error

// probeFrame is the part of an envelope one probe assertion needs. Decoding
// into a partial struct (rather than a map) keeps the frame's shape checkable
// while ignoring every field the probe has no opinion about.
type probeFrame struct {
	Type string `json:"type"`
	Seq  int64  `json:"seq"`
	Rev  int64  `json:"rev"`
	Data struct {
		Role string `json:"role"`
		Code string `json:"code"`
	} `json:"data"`
}

// ProbeURL is the production probe. It joins as role `screen`, which is the
// deliberate choice over the player role: a screen join is not a game mutation
// — no player row, no revision bump, no budget — so a 30-second cadence leaves
// no rubble in the database, and it is the role that must survive the tunnel
// for the projector.
//
// The handshake it demands is the frozen one (D-23): `hello` at seq 1 carrying
// role "screen", then `state.snapshot` at seq 2. Anything else — an error
// frame, a different type, a different sequence — is a failure, and the
// returned error names what arrived. A `websocket: bad handshake` with an HTTP
// 400 is the cloudflared #1652 signature (a dropped `Upgrade` header) and is
// reported verbatim, because it is the single most useful diagnostic line this
// package can produce.
func ProbeURL(ctx context.Context, tunnelURL string) error {
	target, ok := probeURL(tunnelURL)
	if !ok {
		return fmt.Errorf("tunnel: probe: %q is not a usable tunnel origin", tunnelURL)
	}

	// A copy, never the package-level dialer: DefaultDialer is a shared value
	// (the loadtest and every test in the repo dial through it), and mutating it
	// would be a cross-package data race waiting to happen.
	dialer := *websocket.DefaultDialer
	dialer.NetDialContext = (&net.Dialer{}).DialContext

	conn, resp, err := dialer.DialContext(ctx, target, nil)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
			_ = resp.Body.Close()
		}
		return fmt.Errorf("tunnel: probe %s: %w (%s)", target, err, status)
	}

	// Close explicitly, and cleanly: a probe that left a half-open socket behind
	// every 30 seconds would keep the hub's registry non-empty, which is exactly
	// the condition the ticker's quiet-LOBBY early return watches for.
	defer func() {
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second))
		_ = conn.Close()
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
		_ = conn.SetReadDeadline(deadline)
	}

	if err := conn.WriteMessage(websocket.TextMessage, []byte(probeJoin)); err != nil {
		return fmt.Errorf("tunnel: probe %s: sending the screen join: %w", target, err)
	}

	hello, err := readProbeFrame(conn, ctx, target)
	if err != nil {
		return err
	}
	if hello.Type == probeErrorType {
		return probeRefusal(target, hello, "the join")
	}
	if hello.Type != probeHelloType || hello.Seq != 1 || hello.Data.Role != probeScreenRole {
		return fmt.Errorf(
			"tunnel: probe %s: first frame is %q at seq %d with role %q, want %q at seq 1 with role %q (D-23)",
			target, hello.Type, hello.Seq, hello.Data.Role, probeHelloType, probeScreenRole)
	}

	snapshot, err := readProbeFrame(conn, ctx, target)
	if err != nil {
		return err
	}
	if snapshot.Type == probeErrorType {
		return probeRefusal(target, snapshot, "the snapshot")
	}
	if snapshot.Type != probeSnapshotType || snapshot.Seq != 2 {
		return fmt.Errorf(
			"tunnel: probe %s: second frame is %q at seq %d, want %q at seq 2 (D-23)",
			target, snapshot.Type, snapshot.Seq, probeSnapshotType)
	}

	return nil
}

// readProbeFrame reads one frame eagerly. Eagerly is load-bearing: a lazy read
// would leave frames queued on a connection the hub is counting (D-41's drop
// accounting), and it would let the probe finish "healthy" without ever having
// proved the snapshot arrives.
func readProbeFrame(conn *websocket.Conn, ctx context.Context, target string) (probeFrame, error) {
	_, raw, err := conn.ReadMessage()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return probeFrame{}, fmt.Errorf("tunnel: probe %s: %w", target, ctxErr)
		}
		return probeFrame{}, fmt.Errorf("tunnel: probe %s: %w", target, err)
	}
	var frame probeFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		return probeFrame{}, fmt.Errorf("tunnel: probe %s: the frame is not a JSON envelope: %v (%s)", target, err, raw)
	}
	return frame, nil
}

// probeRefusal names a typed refusal: the code is what distinguishes a real
// protocol error from a dead tunnel, so it travels into the message verbatim.
func probeRefusal(target string, frame probeFrame, what string) error {
	return fmt.Errorf("tunnel: probe %s: the server refused %s with code %q at seq %d", target, what, frame.Data.Code, frame.Seq)
}
