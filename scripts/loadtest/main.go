// Command loadtest is the Phase 1 smoke harness behind `make loadtest`: a small
// pool of WebSocket connections that each run a sequential echo round-trip
// against a live /ws, printing one line per connection plus a total, and
// exiting non-zero when any dial fails, any reply mismatches, or a correlation
// is missing.
//
// Scope (D-12): this is a 3-connection liveness smoke, deliberately small. The
// 20-bot rapid-fire harness that asserts revision monotonicity and exercises the
// drop/resync path is Phase 7 work — revisions do not exist yet, so asserting
// ordering here would certify nothing.
//
// It lives inside the main module (`go run ./scripts/loadtest`) and uses the
// same gorilla DefaultDialer the Go tests use, so `make loadtest` needs no
// separate module, no lockfile and no install step (A4).
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// defaultTarget matches `make run` (the binary on its default port).
	defaultTarget = "ws://127.0.0.1:8080/ws"
	// conns is the Phase 1 pool size: enough to prove the handler serves more
	// than one connection, small enough to stay a smoke test.
	conns = 3
	// framesPerConn: frames are sent one at a time per connection, so no
	// ordering guarantee is ever assumed or asserted.
	framesPerConn = 4
	// ioTimeout bounds every read and write so a hung server fails the run
	// instead of blocking it.
	ioTimeout = 5 * time.Second
)

// probe is the payload shape. Note there is no `seq`/revision field: the
// correlation fields are ordinary payload data, not protocol semantics.
type probe struct {
	Probe string `json:"probe"` // "loadtest"
	Conn  int    `json:"conn"`  // connection index, correlates the reply
	Frame int    `json:"frame"` // sequential frame number within the connection
	At    int64  `json:"at"`    // client clock, milliseconds
}

// envelope mirrors the server's echo shape. It is decoded with the exact field
// set the server promises; an extra field would be ignored here, which is why
// the Go test — not this harness — owns the exact-key-set assertion.
type envelope struct {
	Type string          `json:"type"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data"`
}

// result is one connection's outcome, kept in connection order for printing.
type result struct {
	conn int
	ok   int
	errs []string
}

func main() {
	target := resolveTarget()
	start := time.Now()

	results := make([]result, conns)
	var wg sync.WaitGroup
	for i := 0; i < conns; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = runConn(target, i)
		}(i)
	}
	wg.Wait()

	okConns, okFrames, failed := 0, 0, false
	for _, res := range results {
		okFrames += res.ok
		if res.ok == framesPerConn && len(res.errs) == 0 {
			okConns++
			fmt.Printf("conn %d: ok — %d/%d echoes matched\n", res.conn, res.ok, framesPerConn)
			continue
		}
		failed = true
		fmt.Printf("conn %d: FAILED — %d/%d echoes matched\n", res.conn, res.ok, framesPerConn)
		for _, e := range res.errs {
			fmt.Printf("  %s\n", e)
		}
	}
	fmt.Printf("loadtest: %s — %d/%d connections, %d/%d frames matched in %s\n",
		target, okConns, conns, okFrames, conns*framesPerConn, time.Since(start).Round(time.Millisecond))

	if failed {
		os.Exit(1)
	}
}

// resolveTarget picks the first of: positional argument, WS_URL, the default.
func resolveTarget() string {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: go run ./scripts/loadtest [ws-url]\n\n")
		fmt.Fprintf(out, "Dials %d connections against a live /ws echo and exits non-zero on any\n", conns)
		fmt.Fprintf(out, "failure. The URL may also come from WS_URL.\n\n")
		fmt.Fprintf(out, "  default: %s\n", defaultTarget)
	}
	flag.Parse()

	if arg := flag.Arg(0); arg != "" {
		return arg
	}
	if env := os.Getenv("WS_URL"); env != "" {
		return env
	}
	return defaultTarget
}

// runConn dials one connection and sends framesPerConn frames sequentially,
// asserting each reply before sending the next.
func runConn(target string, index int) result {
	res := result{conn: index}

	conn, resp, err := websocket.DefaultDialer.Dial(target, nil)
	if err != nil {
		status := "no response"
		if resp != nil {
			status = resp.Status
			_ = resp.Body.Close()
		}
		res.errs = append(res.errs, fmt.Sprintf("dial %s: %v (%s)", target, err, status))
		return res
	}
	defer func() { _ = conn.Close() }()

	for frame := 1; frame <= framesPerConn; frame++ {
		sent, err := json.Marshal(probe{
			Probe: "loadtest",
			Conn:  index,
			Frame: frame,
			At:    time.Now().UnixMilli(),
		})
		if err != nil {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: marshaling the probe: %v", frame, err))
			return res
		}

		_ = conn.SetWriteDeadline(time.Now().Add(ioTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, sent); err != nil {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: write: %v", frame, err))
			return res
		}

		_ = conn.SetReadDeadline(time.Now().Add(ioTimeout))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: read: %v", frame, err))
			return res
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: envelope is not JSON: %v (%s)", frame, err, raw))
			return res
		}
		switch {
		case env.Type != "echo":
			res.errs = append(res.errs, fmt.Sprintf("frame %d: type = %q, want \"echo\"", frame, env.Type))
			return res
		case env.TS <= 0:
			res.errs = append(res.errs, fmt.Sprintf("frame %d: ts = %d, want a positive server clock", frame, env.TS))
			return res
		case !bytes.Equal(env.Data, sent):
			res.errs = append(res.errs, fmt.Sprintf("frame %d: data = %s, want the payload verbatim %s", frame, env.Data, sent))
			return res
		}

		// Correlation: the echoed payload must be this connection's frame, not
		// another connection's — the difference between "three connections
		// worked" and "three connections shared one good reply".
		var echoed probe
		if err := json.Unmarshal(env.Data, &echoed); err != nil {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: decoding echoed payload: %v", frame, err))
			return res
		}
		if echoed.Conn != index || echoed.Frame != frame || echoed.Probe != "loadtest" {
			res.errs = append(res.errs, fmt.Sprintf("frame %d: correlation mismatch: got conn=%d frame=%d probe=%q",
				frame, echoed.Conn, echoed.Frame, echoed.Probe))
			return res
		}

		res.ok++
	}

	return res
}
