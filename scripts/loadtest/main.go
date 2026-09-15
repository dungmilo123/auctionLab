// Command loadtest is the real-protocol harness behind `make loadtest`: a pair
// of independent player bots that join a live /ws and validate the frozen
// stream contract on the bytes that actually arrive.
//
// Each bot dials, sends `join {role:"player", nickname}`, then reads until its
// own lobby shows both bots. Every frame is checked: `seq` must be contiguous
// from 1 — a duplicate or a gap fails the run, because a gap is the
// client-visible signal of a dropped message (D-21) — and the `rev` carried by
// successive state events must be non-decreasing and contiguous (D-22).
//
// Scope (D-43): two bots, the smallest pool that proves two connections really
// see each other through the hub. The 20-bot rapid-fire harness and the
// stalled-bot convergence drill stay in Phase 7 (REH-01); this is the smoke test
// that runs before every rehearsal.
//
// It lives inside the main module (`go run ./scripts/loadtest`) and uses the
// same gorilla DefaultDialer the Go tests use, so `make loadtest` needs no
// separate module, no lockfile and no install step (A4).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// defaultTarget matches `make run` (the binary on its default port).
	defaultTarget = "ws://127.0.0.1:8080/ws"
	// bots is D-43's Phase 2 pool: two independent players, so each bot can
	// assert that the other one is visible in its own lobby.
	bots = 2
	// maxFrames bounds every bot's read loop. A run that needs more frames than
	// this to see both players is already broken, and an unbounded loop would
	// hang the gate on a server that keeps talking.
	maxFrames = 8
	// ioTimeout bounds every read and write so a hung server fails the run
	// instead of blocking it.
	ioTimeout = 5 * time.Second
)

// envelope is one server→client frame, decoded with the exact five frozen
// fields (D-21). An unknown field would be ignored here on purpose: D-24's
// freeze is additive, and the Go test suite owns the exact-key-set assertion.
type envelope struct {
	Type string          `json:"type"`
	Seq  int64           `json:"seq"`
	Rev  int64           `json:"rev"`
	TS   int64           `json:"ts"`
	Data json.RawMessage `json:"data"`
}

// helloPayload is D-23's private handshake answer. A player's hello is the only
// frame in the protocol that ever carries a token, and only its own (D-26/D-29).
type helloPayload struct {
	Role     string `json:"role"`
	PlayerID string `json:"player"`
	Nickname string `json:"nickname"`
	Token    string `json:"token"`
	Rev      int64  `json:"rev"`
}

// snapshotPayload is the part of the player projection the lobby contract needs
// (D-25): who this bot is, and who else is in the room.
type snapshotPayload struct {
	Phase    string `json:"phase"`
	Revision int64  `json:"revision"`
	Me       struct {
		ID       string `json:"id"`
		Nickname string `json:"nickname"`
		Budget   int64  `json:"budget"`
	} `json:"me"`
	Lobby []lobbyEntry `json:"lobby"`
}

// lobbyEntry is one lobby row: a nickname and its live presence (D-25).
type lobbyEntry struct {
	Nickname  string `json:"nickname"`
	Connected bool   `json:"connected"`
}

// botSpec is one bot's identity and the peers it must be able to see.
type botSpec struct {
	index    int
	nickname string
	peers    []string
}

// result is one bot's outcome, kept in bot order for printing.
type result struct {
	bot  int
	seen int   // frames read
	rev  int64 // the last state-event revision, -1 before the first
	errs []string
}

// ok reports whether this bot met the contract.
func (r result) ok() bool { return len(r.errs) == 0 }

// stream is one bot's view of its own socket: the next `seq` it must see, the
// last `rev` a state event carried, and the lobby names it has seen so far. It
// is deliberately separate from the socket, so the whole D-21/D-22 contract is
// testable without a server.
type stream struct {
	want    int64 // the seq the next frame must carry; 1 at hello
	lastRev int64 // the last revision a state.snapshot carried; -1 before the first
	seen    map[string]bool
}

// newStream starts a stream at its first expected frame.
func newStream() *stream {
	return &stream{want: 1, lastRev: -1, seen: make(map[string]bool)}
}

// observe validates one frame against D-21/D-22/D-23 and folds it in. It
// returns nil only for a frame that fits the stream exactly.
//
// RED sentinel: nothing is validated yet. The GREEN commit implements the
// contract this signature declares.
func (s *stream) observe(env envelope) error {
	_ = env
	return nil
}

// acceptRevision applies D-22's rule to one state event: a revision may repeat
// (sibling messages legitimately share one), but it must never regress or skip.
//
// RED sentinel: the GREEN commit implements the rule.
func (s *stream) acceptRevision(rev int64) error {
	_ = rev
	return nil
}

// complete reports whether every peer has appeared in some snapshot's lobby.
//
// RED sentinel: the GREEN commit reads s.seen.
func (s *stream) complete(peers []string) bool {
	_ = peers
	return false
}

func main() {
	target := resolveTarget()
	start := time.Now()

	// One run, one namespace: the nicknames carry a per-run suffix, so a second
	// `make loadtest` against the same dev database does not collide with the
	// players the previous run left behind (D-28's uniqueness is server-wide).
	run := fmt.Sprintf("lt-%06d", time.Now().UnixNano()%1_000_000)
	nicknames := make([]string, bots)
	specs := make([]botSpec, bots)
	for i := range specs {
		nicknames[i] = fmt.Sprintf("%s-%d", run, i)
	}
	for i := range specs {
		specs[i] = botSpec{index: i, nickname: nicknames[i], peers: nicknames}
	}

	results := make([]result, bots)
	var wg sync.WaitGroup
	for i, spec := range specs {
		wg.Add(1)
		go func(i int, spec botSpec) {
			defer wg.Done()
			results[i] = runBot(target, spec)
		}(i, spec)
	}
	wg.Wait()

	okBots, failed := 0, false
	for _, res := range results {
		if res.ok() {
			okBots++
			fmt.Printf("bot %d: ok — %d frames, last state rev %d, saw every player\n", res.bot, res.seen, res.rev)
			continue
		}
		failed = true
		fmt.Printf("bot %d: FAILED — %d frames read\n", res.bot, res.seen)
		for _, e := range res.errs {
			fmt.Printf("  %s\n", e)
		}
	}
	fmt.Printf("loadtest: %s — %d/%d bots joined and saw each other in %s\n",
		target, okBots, bots, time.Since(start).Round(time.Millisecond))

	if failed {
		os.Exit(1)
	}
}

// resolveTarget picks the first of: positional argument, WS_URL, the default.
func resolveTarget() string {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: go run ./scripts/loadtest [ws-url]\n\n")
		fmt.Fprintf(out, "Dials %d player bots against a live /ws, asserts the real protocol's\n", bots)
		fmt.Fprintf(out, "seq/rev contract, and exits non-zero when either bot cannot join or the\n")
		fmt.Fprintf(out, "other player never becomes visible. The URL may also come from WS_URL.\n\n")
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

// runBot dials one connection, joins as a player, and reads until its own lobby
// shows every peer. It never returns an error: failures are collected in the
// result so one bot's trouble cannot hide another's.
//
// RED sentinel: the dial and the join are real (they are the harness's
// transport contract), but no frame is read or validated yet. The GREEN commit
// implements the read loop.
func runBot(target string, spec botSpec) result {
	res := result{bot: spec.index, rev: -1}

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

	join, err := json.Marshal(map[string]string{
		"type":     "join",
		"role":     "player",
		"nickname": spec.nickname,
	})
	if err != nil {
		res.errs = append(res.errs, fmt.Sprintf("marshaling the join: %v", err))
		return res
	}
	_ = conn.SetWriteDeadline(time.Now().Add(ioTimeout))
	if err := conn.WriteMessage(websocket.TextMessage, join); err != nil {
		res.errs = append(res.errs, fmt.Sprintf("sending the join: %v", err))
		return res
	}

	res.errs = append(res.errs, fmt.Sprintf(
		"no frame was read: the stream contract is not implemented yet (peers: %s)",
		strings.Join(spec.peers, ", ")))
	return res
}
